package main

import (
	"path/filepath"
	"testing"
)

// withStateDir points the persistence layer at a temp directory for one test.
func withStateDir(t *testing.T) {
	t.Helper()
	old := stateDir
	stateDir = t.TempDir()
	t.Cleanup(func() { stateDir = old })
}

func TestDiskMapRoundTrip(t *testing.T) {
	dir := t.TempDir()
	old := stateDir
	stateDir = dir
	defer func() { stateDir = old }()

	m := newDiskMap[int64]("counters")
	m.set("a", 1)
	m.set("b", 2)
	m.update("a", func(cur int64, ok bool) (int64, bool) {
		if !ok {
			t.Fatal("a should exist")
		}
		return cur + 40, true
	})
	if v, _ := m.get("a"); v != 41 {
		t.Fatalf("a=%d", v)
	}

	// A fresh map over the same file reloads what was written.
	reload := newDiskMap[int64]("counters")
	if v, ok := reload.get("a"); !ok || v != 41 {
		t.Fatalf("reload a=%d ok=%v", v, ok)
	}
	if v, _ := reload.get("b"); v != 2 {
		t.Fatalf("reload b=%d", v)
	}
	if _, err := filepath.Glob(filepath.Join(dir, "counters.json")); err != nil {
		t.Fatal(err)
	}
}

func TestDiskMapDeleteAndKeys(t *testing.T) {
	withStateDir(t)
	m := newDiskMap[string]("t")
	m.set("z", "1")
	m.set("a", "2")
	m.delete("z")
	if _, ok := m.get("z"); ok {
		t.Error("z not deleted")
	}
	if got := m.keys(); len(got) != 1 || got[0] != "a" {
		t.Fatalf("keys %v", got)
	}
}

func TestDiskMapMemoryOnly(t *testing.T) {
	old := stateDir
	stateDir = "" // persistence off
	defer func() { stateDir = old }()
	m := newDiskMap[int]("t")
	m.set("k", 5)
	if m.path != "" {
		t.Errorf("memory-only map has a path %q", m.path)
	}
	if v, _ := m.get("k"); v != 5 {
		t.Errorf("k=%d", v)
	}
}
