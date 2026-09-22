package main

// bdStorage user files (service 10): per-account save data.
//
// The game uploads named files for the signed-in account and reads them back when it
// has no local copy. The important one is "account" — a D3.Account.Digest protobuf
// (banner, difficulty unlocks, paragon, seasonal feats). Captured live:
//
//	10/10 uploadUserFile : 10 ctx | 10 filename | 01 overwrite | 13 data
//	10/12 getUserFile    : 10 ctx | 10 filename            -> one 13 data blob
//	10/13                : a listing / get-info variant (answered empty until captured)
//
// We store the uploaded bytes VERBATIM, keyed by account PID + filename, and return
// them unchanged. The server never parses the protobuf, so a save round-trips exactly
// as the console wrote it — no corruption risk. Files persist under D3_USERFILES so a
// save survives a server restart.
//
// The 10/12 read reply is modeled on getPublisherFile (one 13 blob result); it is
// UNCONFIRMED live because a console with a local save never downloads. The write path
// is confirmed and is the valuable half (progress is no longer discarded).

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
)

var (
	userFilesMu  sync.Mutex
	userFilesDir = envOr("D3_USERFILES", "userfiles")
)

// safeName keeps a filename to a single path segment.
func safeName(name string) string {
	name = strings.Map(func(r rune) rune {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9', r == '_', r == '-', r == '.':
			return r
		default:
			return '_'
		}
	}, name)
	name = strings.TrimLeft(name, ".")
	if name == "" {
		name = "file"
	}
	return name
}

func userFilePath(pid uint64, name string) string {
	return filepath.Join(userFilesDir, fmt.Sprintf("%d_%s.bin", pid, safeName(name)))
}

// readBool consumes a tagged bool (01 v) if present; otherwise leaves the offset.
func (r *bdReader) readBool() {
	if r.off < len(r.b) && r.b[r.off] == tagBool {
		r.off += 2
	}
}

func (l *lobbyConn) onStorageUser(task byte, r *bdReader) []byte {
	switch task {
	case 10: // uploadUserFile
		ctx, _ := r.str()
		name, e1 := r.str()
		r.readBool() // overwrite flag
		data, e2 := r.blob()
		pid := l.pid()
		if e1 != nil || e2 != nil || name == "" || pid == 0 {
			l.logf("storage upload ctx=%q unreadable (name=%q err=%v/%v pid=%d)", ctx, name, e1, e2, pid)
			return taskReply(task, 0, nil)
		}
		if err := l.storeUserFile(pid, name, data); err != nil {
			l.logf("storage upload %s: %v", name, err)
			return taskReply(task, 0, nil)
		}
		l.logf("storage upload pid=%d file=%q %d bytes -> saved", pid, name, len(data))
		return taskReply(task, 0, nil)

	case 12: // getUserFile
		_, _ = r.str() // ctx
		name, err := r.str()
		pid := l.pid()
		if err != nil || name == "" || pid == 0 {
			return taskReply(task, 0, nil)
		}
		data := loadUserFile(pid, name)
		if data == nil {
			l.logf("storage read pid=%d file=%q -> none", pid, name)
			return taskReply(task, 0, nil)
		}
		l.logf("storage read pid=%d file=%q -> %d bytes", pid, name, len(data))
		return taskReply(task, 0, func(w *bdWriter) uint32 { w.blobv(data); return 1 })
	}
	// 10/13 and anything else: empty success until its format is captured.
	l.logf("storage task=%d -> empty success", task)
	return taskReply(task, 0, nil)
}

func (l *lobbyConn) storeUserFile(pid uint64, name string, data []byte) error {
	userFilesMu.Lock()
	defer userFilesMu.Unlock()
	if err := os.MkdirAll(userFilesDir, 0o755); err != nil {
		return err
	}
	return os.WriteFile(userFilePath(pid, name), data, 0o644)
}

func loadUserFile(pid uint64, name string) []byte {
	userFilesMu.Lock()
	defer userFilesMu.Unlock()
	b, err := os.ReadFile(userFilePath(pid, name))
	if err != nil {
		return nil
	}
	return b
}
