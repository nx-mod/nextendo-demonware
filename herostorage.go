package main

// bdStorage user files (service 10), the hero/"account" save.
//
// Service 10 also carries getPublisherFile (10/21, handled in services.go). The
// remaining tasks store the player's own data:
//
//	10/10 uploadFile   the game uploads its ~2.5 KB "account" save (a protobuf)
//	10/12, 10/13       read a stored file back (getFile / getUserFiles)
//
// 10/10 was seen live and, like the other upload tasks, returns no result, so
// storing the blob and answering an empty success is correct and safe. The read
// tasks' reply layout is not captured from the Switch client yet, so returning
// the blob is opt-in behind D3_STORAGE_REPLIES=1; off by default it is an empty
// success, which never marks the service unavailable. The store itself is real
// and persisted (herofiles.json), so uploaded heroes survive a restart and can
// be inspected and, once the read layout is captured, served back.

import (
	"strings"
	"time"
)

const (
	storageUploadFile = 10
	storageGetFile    = 12
	storageGetFiles   = 13
)

// heroFile is one stored user file.
type heroFile struct {
	Owner   uint64 `json:"owner"` // Nextendo PID
	Name    string `json:"name"`
	Context string `json:"context,omitempty"`
	Data    []byte `json:"data"` // encoding/json base64-encodes []byte
	Updated int64  `json:"updated"`
}

// heroFiles persists uploads keyed by "<pid>|<context>|<name>".
var heroFiles = newDiskMap[heroFile]("herofiles")

func heroFileKey(pid uint64, context, name string) string {
	return pidKey(pid) + "|" + context + "|" + strings.ToLower(name)
}

// onStorage handles the non-publisher tasks of service 10.
func (l *lobbyConn) onStorage(task byte, r *bdReader) []byte {
	switch task {
	case storageUploadFile:
		if l.player == nil {
			return taskReply(task, 0, nil)
		}
		ctx, name, data, ok := parseStorageUpload(r)
		if !ok {
			l.logf("storage UPLOAD unreadable pid=%d", l.player.PID)
			return taskReply(task, 0, nil)
		}
		heroFiles.set(heroFileKey(l.player.PID, ctx, name), heroFile{
			Owner: l.player.PID, Name: name, Context: ctx, Data: data, Updated: time.Now().Unix(),
		})
		l.logf("storage UPLOAD pid=%d ctx=%q name=%q %d bytes", l.player.PID, ctx, name, len(data))
		return taskReply(task, 0, nil)

	case storageGetFile, storageGetFiles:
		if !framedReplies || l.player == nil {
			return taskReply(task, 0, nil)
		}
		ctx, name, _, _ := parseStorageUpload(r) // a get carries context + name, no blob
		files := heroFilesFor(l.player.PID, ctx, name)
		if len(files) == 0 {
			return taskReply(task, 0, nil)
		}
		l.logf("storage GET pid=%d ctx=%q name=%q -> %d file(s) (framed, unverified)", l.player.PID, ctx, name, len(files))
		return taskReply(task, 0, func(w *bdWriter) uint32 {
			for _, f := range files {
				w.strv(f.Name)
				w.blobv(f.Data)
			}
			return uint32(len(files))
		})
	}
	return taskReply(task, 0, nil)
}

// heroFilesFor returns a player's stored files, optionally filtered by name.
func heroFilesFor(pid uint64, context, name string) []heroFile {
	var out []heroFile
	heroFiles.forEach(func(_ string, f heroFile) {
		if f.Owner != pid {
			return
		}
		if context != "" && f.Context != context {
			return
		}
		if name != "" && !strings.EqualFold(f.Name, name) {
			return
		}
		out = append(out, f)
	})
	return out
}

// parseStorageUpload reads an upload/get request defensively: an optional
// leading context string, an optional file-name string, and (for an upload) the
// trailing blob. Missing pieces are returned empty.
func parseStorageUpload(r *bdReader) (ctx, name string, data []byte, ok bool) {
	if r.off < len(r.b) && r.b[r.off] == tagString {
		ctx, _ = r.str()
	}
	if r.off < len(r.b) && r.b[r.off] == tagString {
		name, _ = r.str()
	}
	if r.off < len(r.b) && r.b[r.off] == tagBlob {
		if b, err := r.blob(); err == nil {
			data = b
		}
	}
	// An upload with neither a name nor data is not usable; a get may have only
	// a context, which is still a valid request.
	ok = ctx != "" || name != "" || len(data) > 0
	return
}
