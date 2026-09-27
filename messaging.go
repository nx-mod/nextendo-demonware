package main

// bdMessaging (service 6), in-game mail.
//
// The game calls 6/14; the season-swap flow (mailing a hero back after a season
// change) needs a mailbox. The Demonware wire layout of the messaging tasks is
// not captured from the Switch client yet, so this frames the feature: a real,
// persisted per-player mailbox (mail.json) with send / list / delete, an HTTP
// admin path on the dashboard to drop a message into a player's box (used by the
// season-swap tooling and for testing), and a best-effort list reply gated
// behind D3_FRAMED_REPLIES. Accepting the call and keeping an empty success is
// the safe default until a capture confirms the layout.

import (
	"sync/atomic"
	"time"
)

const (
	svcMessaging = 6
	msgTask      = 14 // the single messaging task seen in the binary
)

// mailItem is one message in a player's mailbox.
type mailItem struct {
	ID      uint64 `json:"id"`
	From    uint64 `json:"from"` // sender PID, 0 = system
	Subject string `json:"subject,omitempty"`
	Body    []byte `json:"body"` // opaque game payload (base64 in JSON)
	Sent    int64  `json:"sent"`
	Read    bool   `json:"read"`
}

// mailboxes persists each player's mail, keyed by recipient PID string.
var mailboxes = newDiskMap[[]mailItem]("mail")

var mailIDs atomic.Uint64

// sendMail appends a message to a recipient's mailbox.
func sendMail(to uint64, m mailItem) {
	if to == 0 {
		return
	}
	m.ID = mailIDs.Add(1)
	if m.Sent == 0 {
		m.Sent = time.Now().Unix()
	}
	mailboxes.update(pidKey(to), func(cur []mailItem, _ bool) ([]mailItem, bool) {
		return append(cur, m), true
	})
}

// mailFor returns a player's mailbox, oldest first.
func mailFor(pid uint64) []mailItem {
	m, _ := mailboxes.get(pidKey(pid))
	return m
}

// deleteMail removes one message from a player's mailbox; returns whether it was
// present.
func deleteMail(pid, id uint64) bool {
	removed := false
	mailboxes.update(pidKey(pid), func(cur []mailItem, _ bool) ([]mailItem, bool) {
		out := cur[:0]
		for _, m := range cur {
			if m.ID == id {
				removed = true
				continue
			}
			out = append(out, m)
		}
		return out, true
	})
	return removed
}

// onMessaging handles bdMessaging (service 6). Without a captured layout it
// returns the player's mail count on a best-effort basis when D3_FRAMED_REPLIES
// is set, and an empty success otherwise.
func (l *lobbyConn) onMessaging(task byte, r *bdReader) []byte {
	if task != msgTask || l.player == nil {
		return taskReply(task, 0, nil)
	}
	box := mailFor(l.player.PID)
	l.logf("messaging 6/14 pid=%d -> %d message(s) in box", l.player.PID, len(box))
	if !framedReplies {
		return taskReply(task, 0, nil)
	}
	return taskReply(task, 0, func(w *bdWriter) uint32 {
		for _, m := range box {
			w.u64(m.ID)
			w.u64(m.From)
			w.strv(m.Subject)
			w.blobv(m.Body)
		}
		return uint32(len(box))
	})
}
