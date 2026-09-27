package main

import (
	"testing"
)

func TestHeroStorageUploadAndGet(t *testing.T) {
	resetStores(t)
	alice := &playerID{Username: "player1", PID: 1800000101}
	l := &lobbyConn{n: 1, player: alice}

	// upload: context, name, blob
	up := append(rpStr("account"), rpStr("hero.dat")...)
	up = append(up, rpBlob([]byte("save-bytes"))...)
	if n := replyResults(t, l.onStorage(storageUploadFile, &bdReader{b: up})); n != 0 {
		t.Fatalf("upload should be an empty success, got %d", n)
	}
	files := heroFilesFor(alice.PID, "account", "")
	if len(files) != 1 || string(files[0].Data) != "save-bytes" {
		t.Fatalf("stored %+v", files)
	}

	// get is off by default
	get := append(rpStr("account"), rpStr("hero.dat")...)
	if n := replyResults(t, l.onStorage(storageGetFile, &bdReader{b: get})); n != 0 {
		t.Fatalf("framed off, get returned %d", n)
	}
	// framed on: the file comes back
	withFramed(t)
	if n := replyResults(t, l.onStorage(storageGetFile, &bdReader{b: append(rpStr("account"), rpStr("hero.dat")...)})); n != 1 {
		t.Fatalf("framed get rows %d", n)
	}
}

func TestHeroStorageIgnoresAnonymous(t *testing.T) {
	resetStores(t)
	guest := &lobbyConn{n: 9} // no player
	up := append(rpStr("account"), rpStr("hero.dat")...)
	up = append(up, rpBlob([]byte("x"))...)
	l := guest
	replyResults(t, l.onStorage(storageUploadFile, &bdReader{b: up}))
	if heroFiles.len() != 0 {
		t.Error("a player without a PID stored a hero file")
	}
}

func TestCounterIncrement(t *testing.T) {
	resetStores(t)
	l := &lobbyConn{n: 1, player: &playerID{PID: 1}}
	// two counters: id 5 += 3, id 7 += 10
	req := append(rpU32(5), rpU64(3)...)
	req = append(req, rpU32(7)...)
	req = append(req, rpU64(10)...)
	l.onCounter(counterIncrement, &bdReader{b: req})
	if v, _ := counters.get(counterKey(5)); v != 3 {
		t.Fatalf("counter 5 = %d", v)
	}
	// increment again: id 5 += 4 -> 7
	l.onCounter(counterIncrement, &bdReader{b: append(rpU32(5), rpU64(4)...)})
	if v, _ := counters.get(counterKey(5)); v != 7 {
		t.Fatalf("counter 5 = %d", v)
	}
	if tot := counterTotals(); tot["5"] != 7 || tot["7"] != 10 {
		t.Fatalf("totals %v", tot)
	}
}

func TestEventLogAccepts(t *testing.T) {
	resetStores(t)
	before := eventLogTotal.Load()
	l := &lobbyConn{n: 1, player: &playerID{PID: 1}}
	if n := replyResults(t, l.onEventLog(eventLogRecord, &bdReader{b: rpBlob([]byte("telemetry"))})); n != 0 {
		t.Fatalf("event log should be an empty success, got %d", n)
	}
	if eventLogTotal.Load() != before+1 {
		t.Error("event not counted")
	}
}

func TestMailbox(t *testing.T) {
	resetStores(t)
	sendMail(1800000101, mailItem{From: 0, Subject: "welcome", Body: []byte("hi")})
	sendMail(1800000101, mailItem{From: 2, Subject: "hero returned", Body: []byte("data")})
	box := mailFor(1800000101)
	if len(box) != 2 || box[0].Subject != "welcome" || box[0].ID == box[1].ID {
		t.Fatalf("box %+v", box)
	}
	if !deleteMail(1800000101, box[0].ID) {
		t.Fatal("delete reported missing")
	}
	if got := mailFor(1800000101); len(got) != 1 || got[0].Subject != "hero returned" {
		t.Fatalf("after delete %+v", got)
	}
	// sending to nobody is a no-op
	sendMail(0, mailItem{Subject: "void"})
	if mailboxes.len() != 1 {
		t.Fatalf("unexpected mailboxes %v", mailboxes.keys())
	}
}

func TestOnMessagingFramed(t *testing.T) {
	resetStores(t)
	alice := &playerID{Username: "player1", PID: 1800000101}
	l := &lobbyConn{n: 1, player: alice}
	sendMail(alice.PID, mailItem{Subject: "x", Body: []byte("y")})
	if n := replyResults(t, l.onMessaging(msgTask, &bdReader{b: nil})); n != 0 {
		t.Fatalf("framed off, messaging returned %d", n)
	}
	withFramed(t)
	if n := replyResults(t, l.onMessaging(msgTask, &bdReader{b: nil})); n != 1 {
		t.Fatalf("framed messaging rows %d", n)
	}
}

func TestUserDataQuery(t *testing.T) {
	resetStores(t)
	alice := &playerID{Username: "player1", PID: 1800000101}
	bob := &playerID{Username: "player2", PID: 1800000102}
	withOnline(t, map[uint64]*playerID{1: alice, 2: bob})

	la := &lobbyConn{n: 1, player: alice}
	lb := &lobbyConn{n: 2, player: bob}
	// each stores data in context "hero"
	la.onUserData(1, &bdReader{b: append(rpStr("hero"), rpBlob([]byte("A"))...)})
	lb.onUserData(1, &bdReader{b: append(rpStr("hero"), rpBlob([]byte("B"))...)})

	// query off by default
	q := append(rpStr("hero"), rpU32(0)...)
	q = append(q, rpU32(10)...)
	if n := replyResults(t, la.onUserData(11, &bdReader{b: q})); n != 0 {
		t.Fatalf("framed off, query returned %d", n)
	}
	// framed on: both players' rows come back, offset/limit respected
	withFramed(t)
	q = append(rpStr("hero"), rpU32(0)...)
	q = append(q, rpU32(10)...)
	if n := replyResults(t, la.onUserData(11, &bdReader{b: q})); n != 2 {
		t.Fatalf("framed query rows %d, want 2", n)
	}
	q = append(rpStr("hero"), rpU32(1)...) // offset 1
	q = append(q, rpU32(10)...)
	if n := replyResults(t, la.onUserData(11, &bdReader{b: q})); n != 1 {
		t.Fatalf("offset query rows %d, want 1", n)
	}
}
