package main

// bdStats / leaderboards (service 4).
//
// The game calls service 4 (tasks 1, 4, 5, 11, 13; 4/1 and a six-times-at-
// character-select 4/11 seen live) to submit scores and read leaderboards. The
// exact Demonware wire layout of those tasks has NOT been captured from the
// Switch client yet, so this file frames the feature in the parts that are
// knowable and keeps the known-safe answer for the parts that are not:
//
//   - a leaderboard MODEL shaped exactly after Blizzard's own Diablo III Game
//     Data API (season and era leaderboards over REST+JSON): _links, key,
//     column[], row[] with player[] and data[]. That API is the reference for
//     what a leaderboard row means (rank, hero class, paragon, GR level, clear
//     time), so the store and the dashboard export mirror it field for field.
//   - a persisted score STORE (leaderboards.json) keyed by leaderboard + player.
//   - an INGEST path (recordScore) and an HTTP view (/api/leaderboards on the
//     dashboard) so the boards are populated and visible even before the live
//     bdStats capture lands.
//   - a best-effort service-4 HANDLER (onStats) that records the call for the
//     dashboard and, only when D3_FRAMED_REPLIES=1, attempts a typed-buffer
//     reply. Off by default: an unrecognised reply shape can mark the service
//     unavailable in the client, whereas an empty success is harmless.
//
// Filling the remaining gap needs one capture of service 4 from a real session
// (see STATUS.md): the request layout of the submit task and the row layout the
// client's reader expects. The store and export below are ready for it.

import (
	"math"
	"sort"
	"strings"
	"sync"
	"time"
)

const svcStats = 4

// Diablo III leaderboard tasks seen in the binary (NOTES.md §4). Names are
// inferred from the bd SDK generation and the official API's board set; only the
// call sites (not the wire layout) are confirmed.
const (
	statsWriteRow     = 1  // submit this hero's score (seen live 4/1)
	statsFlush        = 4  // commit / acknowledge
	statsGetRows      = 5  // read a page of a leaderboard
	statsGetSelf      = 11 // this player's rank/row (called 6x at character select)
	statsGetByAccount = 13 // rows for a set of accounts
)

// lbEntry is one player's standing on one leaderboard, in the domain terms the
// official API exposes.
type lbEntry struct {
	AccountID uint64 `json:"accountId"` // Nextendo PID
	Name      string `json:"name"`
	HeroClass string `json:"heroClass,omitempty"`
	HeroID    uint64 `json:"heroId,omitempty"`
	Paragon   int64  `json:"paragon,omitempty"`
	RiftLevel int64  `json:"riftLevel,omitempty"`  // Greater Rift tier cleared
	RiftTime  int64  `json:"riftTimeMs,omitempty"` // clear time, milliseconds
	Score     int64  `json:"score"`                // the ranked value
	Season    int    `json:"season,omitempty"`
	Hardcore  bool   `json:"hardcore,omitempty"`
	Updated   int64  `json:"updated"` // unix seconds
}

// leaderboards persists every board as key -> (account PID string -> entry).
var leaderboards = newDiskMap[map[string]lbEntry]("leaderboards")

var lbCallsMu sync.Mutex
var lbCalls = map[byte]int64{} // service-4 task -> times called (dashboard only)

// recordScore inserts or updates a player's standing on a board, keeping the
// better score (higher rift tier; on a tie the faster clear). It is the single
// ingest point: the service-4 handler, a seed, or an HTTP push all go through it.
func recordScore(board string, e lbEntry) {
	board = normalizeBoard(board)
	if board == "" || e.AccountID == 0 {
		return
	}
	e.Updated = time.Now().Unix()
	key := pidKey(e.AccountID)
	leaderboards.update(board, func(cur map[string]lbEntry, ok bool) (map[string]lbEntry, bool) {
		if cur == nil {
			cur = map[string]lbEntry{}
		}
		if prev, exists := cur[key]; exists && !betterScore(e, prev) {
			return cur, true
		}
		cur[key] = e
		return cur, true
	})
}

// betterScore reports whether a ranks strictly above b: a higher ranked score
// wins; equal scores are broken by the higher rift tier, then by the faster
// clear (a zero time means "no clear recorded", which ranks last). Fully tied
// entries return false, so it is a valid ordering for a stable sort.
func betterScore(a, b lbEntry) bool {
	if a.Score != b.Score {
		return a.Score > b.Score
	}
	if a.RiftLevel != b.RiftLevel {
		return a.RiftLevel > b.RiftLevel
	}
	at, bt := a.RiftTime, b.RiftTime
	if at == 0 {
		at = math.MaxInt64
	}
	if bt == 0 {
		bt = math.MaxInt64
	}
	if at != bt {
		return at < bt
	}
	return false
}

// rankedBoard returns a board's entries sorted best-first.
func rankedBoard(board string) []lbEntry {
	m, ok := leaderboards.get(normalizeBoard(board))
	if !ok {
		return nil
	}
	out := make([]lbEntry, 0, len(m))
	for _, e := range m {
		out = append(out, e)
	}
	sort.SliceStable(out, func(i, j int) bool { return betterScore(out[i], out[j]) })
	return out
}

// selfRank returns a player's entry and 1-based rank on a board (rank 0 = absent).
func selfRank(board string, pid uint64) (lbEntry, int) {
	for i, e := range rankedBoard(board) {
		if e.AccountID == pid {
			return e, i + 1
		}
	}
	return lbEntry{}, 0
}

func pidKey(pid uint64) string {
	return strings.TrimSpace(itoa(pid))
}

func itoa(v uint64) string {
	if v == 0 {
		return "0"
	}
	var b [20]byte
	i := len(b)
	for v > 0 {
		i--
		b[i] = byte('0' + v%10)
		v /= 10
	}
	return string(b[i:])
}

// normalizeBoard lower-cases a board key and drops surrounding whitespace, so
// "Rift_Barbarian" and "rift-barbarian" land on the same board.
func normalizeBoard(key string) string {
	key = strings.ToLower(strings.TrimSpace(key))
	return strings.ReplaceAll(key, "_", "-")
}

// noteStatsCall records a service-4 call for the dashboard.
func noteStatsCall(task byte) {
	lbCallsMu.Lock()
	lbCalls[task]++
	lbCallsMu.Unlock()
}

// onStats handles service 4. By default it records the call and returns an
// empty success (the known-safe answer). With D3_STATS_REPLIES=1 it also tries
// to answer read tasks from the store; this path is unverified against the
// client's reader and is opt-in until a capture confirms the layout.
func (l *lobbyConn) onStats(task byte, r *bdReader) []byte {
	noteStatsCall(task)

	// A submit task (4/1) carries the player's score. We parse it defensively:
	// pull the leaderboard context string if present, then the largest plausible
	// integers as rift tier and score. Wrong guesses only mean a board row is
	// not stored; they never corrupt the reply, which stays an empty success.
	if task == statsWriteRow && l.player != nil {
		if e, board, ok := parseStatsSubmit(l, r); ok {
			recordScore(board, e)
			l.logf("stats WRITE board=%q pid=%d score=%d (framed, unverified)", board, e.AccountID, e.Score)
		}
	}

	if !framedReplies {
		return taskReply(task, 0, nil)
	}

	switch task {
	case statsGetSelf:
		if l.player == nil {
			return taskReply(task, 0, nil)
		}
		e, rank := selfRank(defaultBoard, l.player.PID)
		if rank == 0 {
			return taskReply(task, 0, nil)
		}
		return taskReply(task, 0, func(w *bdWriter) uint32 {
			writeStatsRow(w, uint32(rank), e)
			return 1
		})
	case statsGetRows:
		rows := rankedBoard(defaultBoard)
		if len(rows) > 100 {
			rows = rows[:100]
		}
		return taskReply(task, 0, func(w *bdWriter) uint32 {
			for i, e := range rows {
				writeStatsRow(w, uint32(i+1), e)
			}
			return uint32(len(rows))
		})
	}
	return taskReply(task, 0, nil)
}

// defaultBoard is the board a context-free read falls back to until the board
// key is read from the request. It matches the season being served.
const defaultBoard = "rift-overall"

// writeStatsRow writes one leaderboard row as a typed buffer: rank, account id,
// name, rift tier, score. UNVERIFIED against the client reader (see file head).
func writeStatsRow(w *bdWriter, rank uint32, e lbEntry) {
	w.u32(rank)
	w.u64(e.AccountID)
	w.strv(e.Name)
	w.u32(uint32(e.RiftLevel))
	w.u64(uint64(e.Score))
}

// parseStatsSubmit makes a best effort to read a score submission. It reads an
// optional leading context string as the board key, then collects the typed
// integers that follow, mapping the two largest onto rift tier and score.
func parseStatsSubmit(l *lobbyConn, r *bdReader) (lbEntry, string, bool) {
	board := defaultBoard
	if r.off < len(r.b) && r.b[r.off] == tagString {
		if s, err := r.str(); err == nil && s != "" {
			board = s
		}
	}
	var nums []int64
	for r.off < len(r.b) {
		switch r.b[r.off] {
		case tagU32:
			if v, err := r.u32(); err == nil {
				nums = append(nums, int64(v))
			} else {
				r.off = len(r.b)
			}
		case tagU64:
			if v, err := r.u64(); err == nil {
				nums = append(nums, int64(v))
			} else {
				r.off = len(r.b)
			}
		case tagU8:
			_, _ = r.u8()
		case tagString:
			_, _ = r.str()
		case tagBlob:
			_, _ = r.blob()
		default:
			r.off = len(r.b)
		}
	}
	if len(nums) == 0 {
		return lbEntry{}, "", false
	}
	sort.Slice(nums, func(i, j int) bool { return nums[i] > nums[j] })
	e := lbEntry{AccountID: l.player.PID, Name: l.player.Username, Score: nums[0]}
	if len(nums) > 1 {
		e.RiftLevel = nums[1]
	}
	return e, board, true
}
