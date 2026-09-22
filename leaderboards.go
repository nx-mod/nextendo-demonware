package main

// bdStats (service 4): leaderboards.
//
// Requests decoded from a live capture (2026-09-21), see NOTES.md 8b:
//
//	4/11  bulk definitions : 08 u32=1 | 0A u64 ts | 08 u32 count | count x (08 u32 boardID)
//	4/4   board read        : 08 u32 boardID | 0A u64 (1 or ts) | 08 u32 pageSize | 08 u32 offset
//	4/5   board read        : same shape
//	4/13  board read        : 08 u32 boardID | 08 u32 | 0A u64 ts | 08 u32 offset
//
// The 4/4, 4/5, 4/13 tasks are the three view types the game shows (friends / my
// position / top global). Each expects a page of leaderboard entries.
//
// The reply payload is protobuf: the schema was recovered from the game binary
// (D3.Leaderboard, d3hack/capture/nso/protos/Leaderboard.proto). One entry is a
// `Score` message; the required fields are leaderboard_id, scope_id, score, timestamp.
// A `Metadata.Member` carries the displayed name/class/level, and an optional
// HeroSnapshot carries gear/skills for the inspect view (omitted here).
//
// STATUS: TRIAL. Each result is written as one length-delimited value (a `Score`
// blob) and numResults is the entry count — this matches how other services return
// per-row results, but the exact wrapping is UNCONFIRMED until a live board renders
// it. Returning zero results is always safe (the board shows empty), so a wrong guess
// degrades to today's behaviour rather than corrupting anything.

const svcStats = 4

// LeaderboardScores { repeated Score scores = 1 } - the board-read reply message.
const lbScoresScores = 1

// Score field numbers (D3.Leaderboard.Score).
const (
	scoreLeaderboardID    = 1 // uint64
	scoreScopeID          = 2 // uint32
	scoreScore            = 3 // fixed64
	scoreTimestamp        = 4 // fixed64
	scoreMetadata         = 5 // Metadata
	scoreGameAccountID    = 6 // uint64
	scoreScoreBand        = 7 // uint32
	scoreScorePlayerCount = 8 // uint32
)

// Metadata / Member field numbers.
const (
	metaTeamMember  = 10 // repeated Member
	memberAccountID = 1  // uint64
	memberHeroName  = 2  // string
	memberHeroClass = 3  // fixed32
	memberHeroLevel = 12 // uint32
	memberBattleTag = 10 // string
)

// buildScore serializes one D3.Leaderboard.Score. name/class/level go into a single
// team Member so the row shows something; score is the ranked value (e.g. a GR tier).
func buildScore(boardID, scopeID uint64, score, ts uint64, accountID uint64, name string, class uint32, level uint32) []byte {
	var p pb
	p.u64always(scoreLeaderboardID, boardID)
	p.u64always(scoreScopeID, scopeID)
	p.fixed64(scoreScore, score)
	p.fixed64(scoreTimestamp, ts)
	p.u64(scoreGameAccountID, accountID)
	p.message(scoreMetadata, func(m *pb) {
		m.message(metaTeamMember, func(mem *pb) {
			mem.u64(memberAccountID, accountID)
			mem.str(memberHeroName, name)
			if class != 0 {
				mem.fixed32(memberHeroClass, class)
			}
			mem.u64always(memberHeroLevel, uint64(level))
			mem.str(memberBattleTag, name)
		})
	})
	return p.b
}

func (l *lobbyConn) onStats(task byte, r *bdReader) []byte {
	switch task {
	case 4, 5, 13:
		// board read: first typed field is the board id; the rest is paging.
		boardID, err := r.u32()
		if err != nil {
			l.logf("stats board read task=%d: unreadable board id", task)
			return taskReply(task, 0, nil)
		}
		// Board reads (the three view types). Returning entries needs the exact reply
		// format the game's result-reader expects; four wire-format guesses were all
		// rejected (blob/struct, bare/wrapped, with/without the count envelope), so
		// until the reader is decoded from the binary we return an empty (but valid,
		// non-erroring) result. This is what the board showed before — safe.
		l.logf("stats board read task=%d board=%d -> empty (reply format TBD)", task, boardID)
		return taskReply(task, 0, nil)
	}
	// 4/11 (bulk definitions) and anything else: empty success, as before.
	l.logf("stats task=%d -> empty success", task)
	return taskReply(task, 0, nil)
}
