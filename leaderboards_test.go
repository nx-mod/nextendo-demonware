package main

import (
	"encoding/json"
	"testing"
)

// replyResults decodes a task reply header and returns the result count.
func replyResults(t *testing.T, reply []byte) uint32 {
	t.Helper()
	r := &bdReader{b: reply}
	if _, err := r.u64(); err != nil {
		t.Fatal(err)
	}
	code, err := r.u32()
	if err != nil || code != 0 {
		t.Fatalf("error code %d %v", code, err)
	}
	if _, err := r.u8(); err != nil {
		t.Fatal(err)
	}
	n, err := r.u32()
	if err != nil {
		t.Fatal(err)
	}
	return n
}

func TestRecordScoreKeepsBest(t *testing.T) {
	resetStores(t)
	recordScore("rift-barbarian", lbEntry{AccountID: 100, Name: "a", RiftLevel: 90, Score: 90})
	recordScore("rift-barbarian", lbEntry{AccountID: 100, Name: "a", RiftLevel: 80, Score: 80}) // worse, ignored
	recordScore("rift-barbarian", lbEntry{AccountID: 200, Name: "b", RiftLevel: 95, Score: 95, RiftTime: 50000})
	recordScore("rift-barbarian", lbEntry{AccountID: 300, Name: "c", RiftLevel: 95, Score: 95, RiftTime: 100000}) // ties b's tier but slower clear

	rows := rankedBoard("rift-barbarian")
	if len(rows) != 3 {
		t.Fatalf("rows %d", len(rows))
	}
	if rows[0].AccountID != 200 || rows[1].AccountID != 300 || rows[2].AccountID != 100 {
		t.Fatalf("order %v", []uint64{rows[0].AccountID, rows[1].AccountID, rows[2].AccountID})
	}
	if e, rank := selfRank("rift-barbarian", 100); rank != 3 || e.Score != 90 {
		t.Fatalf("self a rank=%d score=%d", rank, e.Score)
	}
	if _, rank := selfRank("rift-barbarian", 999); rank != 0 {
		t.Fatalf("absent player got rank %d", rank)
	}
}

func TestRecordScoreNormalizesBoard(t *testing.T) {
	resetStores(t)
	recordScore("Rift_Wizard", lbEntry{AccountID: 1, Score: 10})
	recordScore("rift-wizard", lbEntry{AccountID: 2, Score: 20})
	if got := rankedBoard("RIFT-WIZARD"); len(got) != 2 {
		t.Fatalf("normalize failed: %d rows", len(got))
	}
	if _, ok := leaderboards.get(""); ok {
		t.Error("empty board key stored")
	}
	recordScore("", lbEntry{AccountID: 1, Score: 1})  // no board -> dropped
	recordScore("x", lbEntry{AccountID: 0, Score: 1}) // no player -> dropped
	if leaderboards.len() != 1 {
		t.Fatalf("unexpected boards: %v", leaderboards.keys())
	}
}

func TestLeaderboardOfficialSchema(t *testing.T) {
	resetStores(t)
	recordScore("rift-barbarian", lbEntry{AccountID: 100, Name: "a", HeroClass: "barbarian", Paragon: 800, RiftLevel: 90, RiftTime: 540000, Season: 37})
	doc := buildLeaderboardDoc("rift-barbarian")

	// re-marshal and check the exact field names the Blizzard API uses
	raw, _ := json.Marshal(doc)
	var back map[string]any
	if err := json.Unmarshal(raw, &back); err != nil {
		t.Fatal(err)
	}
	for _, key := range []string{"_links", "key", "title", "column", "row", "last_update_time", "generated_by", "season"} {
		if _, ok := back[key]; !ok {
			t.Errorf("missing top-level field %q", key)
		}
	}
	if doc.Key != "rift-barbarian" || doc.Season != 37 || len(doc.Row) != 1 {
		t.Fatalf("doc %+v", doc)
	}
	row := doc.Row[0]
	if row.Order != 0 || len(row.Player) != 1 || row.Player[0].AccountID != 100 {
		t.Fatalf("row %+v", row)
	}
	// the data cells carry the game's leaderboard columns
	byID := map[string]lbValue{}
	for _, d := range row.Data {
		byID[d.ID] = d
	}
	if byID["Rank"].Number != 1 || byID["GreaterRiftLevel"].Number != 90 || byID["Paragon"].Number != 800 {
		t.Fatalf("data cells %+v", byID)
	}
	if byID["HeroClass"].String != "barbarian" || byID["Completed"].Timestamp == 0 {
		t.Fatalf("data cells %+v", byID)
	}
}

func TestSeasonIndexListsKnownBoards(t *testing.T) {
	resetStores(t)
	idx := buildSeasonIndex(37)
	if idx.SeasonID != 37 || len(idx.Leaderboard) < len(knownBoards) {
		t.Fatalf("index %d boards, season %d", len(idx.Leaderboard), idx.SeasonID)
	}
	// a board with scores that is not in the known set still shows up
	recordScore("rift-custom", lbEntry{AccountID: 1, Score: 1})
	found := false
	for _, ref := range buildSeasonIndex(37).Leaderboard {
		if ref.Key == "rift-custom" {
			found = true
		}
	}
	if !found {
		t.Error("custom board not listed")
	}
}

func TestOnStatsRecordsAndReplies(t *testing.T) {
	resetStores(t)
	alice := &playerID{Username: "player1", PID: 1800000101}
	l := &lobbyConn{n: 1, player: alice}

	// submit: context board, then two integers (score, tier)
	req := append(rpStr("rift-wizard"), rpU32(95)...)
	req = append(req, rpU32(88)...)
	l.onStats(statsWriteRow, &bdReader{b: req})
	if e, rank := selfRank("rift-wizard", alice.PID); rank != 1 || e.Score != 95 || e.RiftLevel != 88 {
		t.Fatalf("submit stored e=%+v rank=%d", e, rank)
	}

	// off by default: getSelf returns an empty success
	if n := replyResults(t, l.onStats(statsGetSelf, &bdReader{b: rpStr("rift-wizard")})); n != 0 {
		t.Fatalf("framed off, got %d rows", n)
	}

	// framed on: getSelf reads the default board (read tasks carry no captured
	// board key), so a row there is returned.
	withFramed(t)
	recordScore(defaultBoard, lbEntry{AccountID: alice.PID, Name: alice.Username, Score: 50})
	if n := replyResults(t, l.onStats(statsGetSelf, &bdReader{b: nil})); n != 1 {
		t.Fatalf("framed getSelf rows %d", n)
	}
}
