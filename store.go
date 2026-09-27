package main

// Persistence.
//
// The server kept every stateful thing (user data, hero uploads, counters,
// mail, leaderboards) in a plain map that was lost on restart. diskMap is a
// small standard-library key/value store that keeps those maps on disk as one
// JSON file each, so state survives a restart.
//
// D3_STATE picks the directory (default "state"); "off" or "" keeps everything
// in memory only, which is what the tests and a throwaway stack want. A disk
// error is never fatal: the store logs it and carries on in memory, so a
// read-only or full disk degrades to the old behaviour instead of dropping the
// lobby.
//
// Writes are atomic (write a temporary file, then rename) so a crash mid-flush
// cannot leave a half-written file. Every map guards itself with its own mutex.

import (
	"encoding/json"
	"log"
	"os"
	"path/filepath"
	"sort"
	"sync"
)

// stateDir is the directory the persisted maps live in, or "" for memory only.
var stateDir = resolveStateDir(envOr("D3_STATE", "state"))

// resolveStateDir returns the state directory, or "" when persistence is off or
// the directory cannot be created.
func resolveStateDir(dir string) string {
	if dir == "" || dir == "off" || dir == "0" {
		return ""
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		log.Printf("[D3 State] %s unusable, running in memory only: %v", dir, err)
		return ""
	}
	return dir
}

// diskMap is a string-keyed map persisted to one JSON file. V must be a type
// encoding/json can round-trip. When path is "", it never touches the disk.
type diskMap[V any] struct {
	mu   sync.Mutex
	path string
	m    map[string]V
}

// newDiskMap loads <stateDir>/<name>.json, or starts empty. name is a bare file
// name; with persistence off the map is memory only.
func newDiskMap[V any](name string) *diskMap[V] {
	d := &diskMap[V]{m: map[string]V{}}
	if stateDir == "" {
		return d
	}
	d.path = filepath.Join(stateDir, name+".json")
	raw, err := os.ReadFile(d.path)
	if err != nil {
		if !os.IsNotExist(err) {
			log.Printf("[D3 State] read %s: %v", d.path, err)
		}
		return d
	}
	if err := json.Unmarshal(raw, &d.m); err != nil {
		log.Printf("[D3 State] parse %s: %v (starting empty)", d.path, err)
		d.m = map[string]V{}
		return d
	}
	log.Printf("[D3 State] loaded %s (%d entries)", d.path, len(d.m))
	return d
}

func (d *diskMap[V]) get(key string) (V, bool) {
	d.mu.Lock()
	defer d.mu.Unlock()
	v, ok := d.m[key]
	return v, ok
}

func (d *diskMap[V]) set(key string, v V) {
	d.mu.Lock()
	d.m[key] = v
	d.flushLocked()
	d.mu.Unlock()
}

// update mutates the value for key under the lock and persists the result. fn
// receives the current value (zero value if absent) and whether it existed, and
// returns the value to store and whether to keep it (false deletes it).
func (d *diskMap[V]) update(key string, fn func(cur V, ok bool) (V, bool)) V {
	d.mu.Lock()
	defer d.mu.Unlock()
	cur, ok := d.m[key]
	next, keep := fn(cur, ok)
	if keep {
		d.m[key] = next
	} else {
		delete(d.m, key)
	}
	d.flushLocked()
	return next
}

func (d *diskMap[V]) delete(key string) {
	d.mu.Lock()
	delete(d.m, key)
	d.flushLocked()
	d.mu.Unlock()
}

// keys returns the map's keys, sorted, for stable listings and tests.
func (d *diskMap[V]) keys() []string {
	d.mu.Lock()
	defer d.mu.Unlock()
	out := make([]string, 0, len(d.m))
	for k := range d.m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

func (d *diskMap[V]) len() int {
	d.mu.Lock()
	defer d.mu.Unlock()
	return len(d.m)
}

// forEach calls fn for every entry under the lock. fn must not call back into
// the same map.
func (d *diskMap[V]) forEach(fn func(key string, v V)) {
	d.mu.Lock()
	defer d.mu.Unlock()
	for k, v := range d.m {
		fn(k, v)
	}
}

// flushLocked writes the map atomically. The caller holds the mutex. A memory-
// only map (path == "") does nothing.
func (d *diskMap[V]) flushLocked() {
	if d.path == "" {
		return
	}
	raw, err := json.Marshal(d.m)
	if err != nil {
		log.Printf("[D3 State] encode %s: %v", d.path, err)
		return
	}
	tmp := d.path + ".tmp"
	if err := os.WriteFile(tmp, raw, 0o644); err != nil {
		log.Printf("[D3 State] write %s: %v", tmp, err)
		return
	}
	if err := os.Rename(tmp, d.path); err != nil {
		log.Printf("[D3 State] rename %s: %v", d.path, err)
	}
}
