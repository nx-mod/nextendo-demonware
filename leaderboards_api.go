package main

// Leaderboards, in Blizzard's own Diablo III Game Data API shape.
//
// Blizzard exposes season and era leaderboards over plain REST+JSON:
//
//	GET /data/d3/season/                              season index
//	GET /data/d3/season/{id}                          the season's board list
//	GET /data/d3/season/{id}/leaderboard/{board}      one leaderboard
//	GET /data/d3/era/{id}/leaderboard/{board}         non-seasonal (era) board
//
// A leaderboard is { _links, key, title, column[], row[], last_update_time,
// generated_by, achievement_points, season }. A row is { player[], order,
// data[] }; a data value is { id, timestamp, number, string }; a column is
// { id, hidden, order, label, type }. This file reproduces that structure from
// the internal store (leaderboards.go), so the boards can be read by anything
// that speaks the official API and rendered by the dashboard. The values match
// the game's own leaderboard columns: Rank, GreaterRiftLevel, RiftTime, Hero
// class and level, Paragon.
//
// Served read-only by the dashboard on /api/leaderboards (see dashboard.go).

import (
	"strings"
	"time"
)

type lbHref struct {
	Href string `json:"href"`
}
type lbLinks struct {
	Self lbHref `json:"self"`
}

// lbLabel is the localized label the API attaches to a column.
type lbLabel struct {
	EnUS string `json:"en_US"`
}
type lbColType struct {
	ID string `json:"id"`
}
type lbColumn struct {
	ID     string    `json:"id"`
	Hidden bool      `json:"hidden"`
	Order  int       `json:"order,omitempty"`
	Label  lbLabel   `json:"label"`
	Type   lbColType `json:"type"`
}

// lbValue is one { id, timestamp, number, string } data cell. Only the field
// that carries the value for that column id is set.
type lbValue struct {
	ID        string `json:"id"`
	Timestamp int64  `json:"timestamp,omitempty"`
	Number    int64  `json:"number,omitempty"`
	String    string `json:"string,omitempty"`
}
type lbPlayer struct {
	AccountID uint64    `json:"accountId"`
	Data      []lbValue `json:"data,omitempty"`
}
type lbRow struct {
	Player []lbPlayer `json:"player"`
	Order  int        `json:"order"`
	Data   []lbValue  `json:"data"`
}

// lbGeneratedBy mirrors the API's generator stamp.
type lbGeneratedBy struct {
	Type string `json:"type"`
}

// leaderboardDoc is the top-level object the API returns for one board.
type leaderboardDoc struct {
	Links             lbLinks       `json:"_links"`
	Key               string        `json:"key"`
	Title             lbLabel       `json:"title"`
	Column            []lbColumn    `json:"column"`
	Row               []lbRow       `json:"row"`
	LastUpdateTime    string        `json:"last_update_time"`
	GeneratedBy       lbGeneratedBy `json:"generated_by"`
	AchievementPoints bool          `json:"achievement_points"`
	Season            int           `json:"season"`
}

// standardColumns is the column set the game's rift boards use, in display order.
var standardColumns = []lbColumn{
	{ID: "Rank", Order: 1, Label: lbLabel{"Rank"}, Type: lbColType{"number"}},
	{ID: "HeroClass", Order: 2, Label: lbLabel{"Class"}, Type: lbColType{"string"}},
	{ID: "HeroLevel", Order: 3, Hidden: true, Label: lbLabel{"Level"}, Type: lbColType{"number"}},
	{ID: "Paragon", Order: 4, Label: lbLabel{"Paragon"}, Type: lbColType{"number"}},
	{ID: "GreaterRiftLevel", Order: 5, Label: lbLabel{"Rift Level"}, Type: lbColType{"number"}},
	{ID: "RiftTime", Order: 6, Label: lbLabel{"Clear Time"}, Type: lbColType{"number"}},
	{ID: "Completed", Order: 7, Label: lbLabel{"Completed"}, Type: lbColType{"timestamp"}},
}

// buildLeaderboardDoc renders one board in the official schema.
func buildLeaderboardDoc(board string) leaderboardDoc {
	board = normalizeBoard(board)
	entries := rankedBoard(board)
	rows := make([]lbRow, 0, len(entries))
	season := 0
	for i, e := range entries {
		if e.Season != 0 {
			season = e.Season
		}
		rows = append(rows, lbRow{
			Player: []lbPlayer{{AccountID: e.AccountID, Data: []lbValue{
				{ID: "HeroClass", String: e.HeroClass},
			}}},
			Order: i,
			Data: []lbValue{
				{ID: "Rank", Number: int64(i + 1)},
				{ID: "HeroClass", String: e.HeroClass},
				{ID: "HeroLevel", Number: int64(e.HeroID)},
				{ID: "Paragon", Number: e.Paragon},
				{ID: "GreaterRiftLevel", Number: e.RiftLevel},
				{ID: "RiftTime", Number: e.RiftTime},
				{ID: "Completed", Timestamp: e.Updated * 1000},
			},
		})
	}
	return leaderboardDoc{
		Links:          lbLinks{Self: lbHref{Href: "/api/leaderboards/" + board}},
		Key:            board,
		Title:          lbLabel{boardTitle(board)},
		Column:         standardColumns,
		Row:            rows,
		LastUpdateTime: time.Now().UTC().Format(time.RFC3339),
		GeneratedBy:    lbGeneratedBy{Type: "nextendo-demonware"},
		Season:         season,
	}
}

// boardTitle turns a board key into a readable title, matching the API's naming
// (rift-barbarian -> "Barbarian", hardcore-rift-team-2 -> "Hardcore Team 2").
func boardTitle(board string) string {
	t := strings.ReplaceAll(board, "-", " ")
	t = strings.TrimPrefix(t, "rift ")
	t = strings.Replace(t, "hardcore rift ", "hardcore ", 1)
	words := strings.Fields(t)
	for i, w := range words {
		words[i] = strings.ToUpper(w[:1]) + w[1:]
	}
	return strings.Join(words, " ")
}

// knownBoards is the standard Diablo III board set: per-class Greater Rift
// boards, team boards and achievement points, each with a hardcore variant. It
// seeds the season index so the boards exist before any score arrives.
var knownBoards = func() []string {
	base := []string{
		"rift-barbarian", "rift-crusader", "rift-dh", "rift-monk",
		"rift-necromancer", "rift-wd", "rift-wizard",
		"rift-team-2", "rift-team-3", "rift-team-4",
		"achievement-points",
	}
	out := make([]string, 0, len(base)*2)
	for _, b := range base {
		out = append(out, b, "hardcore-"+b)
	}
	return out
}()

// seasonIndexDoc is the board list for a season (or era).
type seasonIndexDoc struct {
	Links          lbLinks          `json:"_links"`
	SeasonID       int              `json:"season_id"`
	LastUpdateTime string           `json:"last_update_time"`
	GeneratedBy    lbGeneratedBy    `json:"generated_by"`
	Leaderboard    []seasonBoardRef `json:"leaderboard"`
}
type seasonBoardRef struct {
	Key   string  `json:"leaderboard"`
	Links lbLinks `json:"_links"`
}

// buildSeasonIndex lists every board that exists for a season: the standard set
// plus any board that already has scores.
func buildSeasonIndex(season int) seasonIndexDoc {
	seen := map[string]bool{}
	var refs []seasonBoardRef
	add := func(b string) {
		b = normalizeBoard(b)
		if b == "" || seen[b] {
			return
		}
		seen[b] = true
		refs = append(refs, seasonBoardRef{
			Key:   b,
			Links: lbLinks{Self: lbHref{Href: "/api/leaderboards/" + b}},
		})
	}
	for _, b := range knownBoards {
		add(b)
	}
	for _, b := range leaderboards.keys() {
		add(b)
	}
	return seasonIndexDoc{
		Links:          lbLinks{Self: lbHref{Href: "/api/leaderboards"}},
		SeasonID:       season,
		LastUpdateTime: time.Now().UTC().Format(time.RFC3339),
		GeneratedBy:    lbGeneratedBy{Type: "nextendo-demonware"},
		Leaderboard:    refs,
	}
}
