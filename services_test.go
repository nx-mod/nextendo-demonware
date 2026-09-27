package main

import (
	"os"
	"testing"
)

// TestMain forces the persisted service maps to be memory-only for the whole
// test binary, so tests never write into ./state. Round-trip tests that need a
// real directory set stateDir themselves (store_test.go).
func TestMain(m *testing.M) {
	stateDir = ""
	os.RemoveAll("state") // remove the empty dir package init created
	resetStoresGlobal()
	os.Exit(m.Run())
}

// resetStoresGlobal reinitializes every persisted global as memory-only.
func resetStoresGlobal() {
	leaderboards = newDiskMap[map[string]lbEntry]("leaderboards")
	heroFiles = newDiskMap[heroFile]("herofiles")
	counters = newDiskMap[int64]("counters")
	dmlData = newDiskMap[[]byte]("dml")
	mailboxes = newDiskMap[[]mailItem]("mail")
	userData = newDiskMap[[]byte]("userdata")
}

// resetStores gives one test a clean set of memory-only stores.
func resetStores(t *testing.T) {
	t.Helper()
	old := stateDir
	stateDir = ""
	resetStoresGlobal()
	t.Cleanup(func() { stateDir = old })
}

// withFramed turns the best-effort typed replies on for one test.
func withFramed(t *testing.T) {
	t.Helper()
	old := framedReplies
	framedReplies = true
	t.Cleanup(func() { framedReplies = old })
}
