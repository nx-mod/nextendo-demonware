package main

import (
	"testing"
	"time"
)

// resetSessions clears the shared matchmaking table for a test.
func resetSessions(t *testing.T) {
	t.Helper()
	sessionsMu.Lock()
	sessions = map[[8]byte]*mmSession{}
	sessionsMu.Unlock()
}

func addSession(id byte, ownerPID uint64, num, max uint32, updated time.Time) {
	var sid [8]byte
	sid[0] = id
	sessionsMu.Lock()
	sessions[sid] = &mmSession{
		context: "d3", owner: 99, ownerPID: ownerPID,
		numPlayers: num, maxPlayers: max, updated: updated,
		hostAddr: make([]byte, 67),
	}
	sessionsMu.Unlock()
}

func findReq() []byte {
	req := append(rpU32(0), rpU32(0)...) // query, start
	return append(req, rpU32(50)...)     // max
}

func TestFindSkipsFullAndStaleGames(t *testing.T) {
	resetSessions(t)
	now := time.Now()
	addSession(1, 1001, 1, 4, now)                 // open: 1/4
	addSession(2, 1002, 4, 4, now)                 // full: 4/4, unjoinable
	addSession(3, 1003, 1, 4, now.Add(-time.Hour)) // stale: host silent past the TTL

	l := &lobbyConn{n: 1, player: &playerID{PID: 2000}}
	if n := replyResults(t, l.onMatchMaking(mmFindSessions, &bdReader{b: findReq()})); n != 1 {
		t.Fatalf("find returned %d games, want 1 (open only)", n)
	}
}

func TestStaleAndFullHelpers(t *testing.T) {
	now := time.Now()
	open := &mmSession{owner: 1, numPlayers: 1, maxPlayers: 4, updated: now}
	if !open.joinable(now) || open.full() || open.stale(now) {
		t.Errorf("open game not joinable: %+v", open)
	}
	full := &mmSession{owner: 1, numPlayers: 4, maxPlayers: 4, updated: now}
	if full.joinable(now) || !full.full() {
		t.Error("full game reported joinable")
	}
	// an orphaned host (owner 0) is never stale here: the orphan grace handles it
	orphan := &mmSession{owner: 0, numPlayers: 1, maxPlayers: 4, updated: now.Add(-time.Hour)}
	if orphan.stale(now) {
		t.Error("orphan should be reaped by orphanGrace, not the stale sweep")
	}
	// with the sweep disabled nothing goes stale
	old := sessionTTL
	sessionTTL = 0
	if (&mmSession{owner: 1, updated: now.Add(-24 * time.Hour)}).stale(now) {
		t.Error("stale sweep should be off when sessionTTL==0")
	}
	sessionTTL = old
}

func TestReapRemovesStale(t *testing.T) {
	resetSessions(t)
	now := time.Now()
	addSession(1, 1001, 1, 4, now)                 // fresh
	addSession(2, 1002, 1, 4, now.Add(-time.Hour)) // stale
	reapSessionsOnce()
	sessionsMu.Lock()
	n := len(sessions)
	sessionsMu.Unlock()
	if n != 1 {
		t.Fatalf("after reap %d sessions, want 1", n)
	}
}

func TestRequireTicketDropsZeroKey(t *testing.T) {
	l := &lobbyConn{sessDir: t.TempDir()}
	old := requireTicket
	defer func() { requireTicket = old }()

	requireTicket = false
	if got := l.candidates(nil); len(got) != 1 || got[0].name != "zero" {
		t.Fatalf("default candidates %+v, want [zero]", got)
	}
	requireTicket = true
	if got := l.candidates(nil); len(got) != 0 {
		t.Fatalf("require-ticket candidates %+v, want none", got)
	}
}

func TestRichPresenceResolvesConsoleFriendID(t *testing.T) {
	alice := &playerID{Username: "consoleguy", PID: 1800000200}
	withOnline(t, map[uint64]*playerID{1: alice})

	// a Nintendo device id mapped to alice's nickname, as the baas-proxy log
	// would supply. Freeze aliasLoaded so refreshAliases does not clear it.
	deviceID := uint64(0x0123456789ABCDEF)
	aliasMu.Lock()
	oldAliases, oldLoaded := aliases, aliasLoaded
	aliases = map[uint64]string{deviceID: "consoleguy"}
	aliasLoaded = time.Now()
	aliasMu.Unlock()
	t.Cleanup(func() {
		aliasMu.Lock()
		aliases, aliasLoaded = oldAliases, oldLoaded
		aliasMu.Unlock()
	})

	la := &lobbyConn{n: 1, player: alice}
	la.onRichPresence(rpSet, &bdReader{b: rpSetRequest("", 0, "", 1, []byte("boss"))})

	// a friend asks by alice's CONSOLE device id, not her PID
	lb := &lobbyConn{n: 2, player: &playerID{PID: 999}}
	rows, ids := rpRows(t, lb.onRichPresence(rpGet, &bdReader{b: rpGetRequest("", deviceID)}))
	if len(rows) != 1 || string(rows[0].data) != "boss" || ids[0] != deviceID {
		t.Fatalf("console friend id not resolved: rows=%+v ids=%v", rows, ids)
	}
}
