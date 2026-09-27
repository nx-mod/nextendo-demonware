package main

// The smaller Demonware services the game touches on connect and that the
// server previously answered with a bare empty success:
//
//	23 bdCounter  1  incrementCounters(u32 counter, i64 delta) -> shared totals
//	27 bdDML      2  getUserData() -> title-wide key/value blob (no args)
//	67 bdEventLog 6  recordTaggedEvents / logEvents -> telemetry sink
//
// Counters are real and persisted (counters.json): the game increments a set of
// shared totals and the dashboard shows them. bdDML returns a server-owned blob
// (empty until content is configured). bdEventLog is a sink: the payload is
// logged for a capture and discarded. As elsewhere, the typed replies for the
// tasks whose wire layout is not captured are gated behind D3_FRAMED_REPLIES; the
// state-keeping (the counter increment, the event count) always happens.

import (
	"sync/atomic"
)

const (
	svcCounter  = 23
	svcDML      = 27
	svcEventLog = 67

	counterIncrement = 1
	dmlGetUserData   = 2
	eventLogRecord   = 6
)

// counters persists shared totals keyed by the counter id the game sends.
var counters = newDiskMap[int64]("counters")

var eventLogTotal atomic.Int64 // events accepted by bdEventLog (dashboard)

// onCounter handles bdCounter (service 23). 23/1 increments one or more
// counters by a signed delta and, in the SDK, returns their new values.
func (l *lobbyConn) onCounter(task byte, r *bdReader) []byte {
	if task != counterIncrement {
		return taskReply(task, 0, nil)
	}
	type bump struct {
		id  uint32
		val int64
	}
	var bumps []bump
	// The request is a run of (u32 counter id, i64/u64 delta) pairs. We read
	// defensively: a u32 followed by any integer tag.
	for r.off < len(r.b) {
		if r.b[r.off] != tagU32 {
			break
		}
		id, err := r.u32()
		if err != nil {
			break
		}
		delta, ok := readAnyInt(r)
		if !ok {
			break
		}
		newVal := counters.update(counterKey(id), func(cur int64, _ bool) (int64, bool) {
			return cur + delta, true
		})
		bumps = append(bumps, bump{id, newVal})
	}
	if len(bumps) == 0 {
		return taskReply(task, 0, nil)
	}
	l.logf("counter INCREMENT %d counter(s)", len(bumps))
	if !framedReplies {
		return taskReply(task, 0, nil)
	}
	return taskReply(task, 0, func(w *bdWriter) uint32 {
		for _, b := range bumps {
			w.u32(b.id)
			w.u64(uint64(b.val))
		}
		return uint32(len(bumps))
	})
}

func counterKey(id uint32) string { return itoa(uint64(id)) }

// counterTotals returns the persisted counters for the dashboard.
func counterTotals() map[string]int64 {
	out := map[string]int64{}
	counters.forEach(func(k string, v int64) { out[k] = v })
	return out
}

// readAnyInt reads a signed/unsigned integer of any width, for fields whose tag
// varies between captures (i32 07, u32 08, i64 09, u64 0A).
func readAnyInt(r *bdReader) (int64, bool) {
	if r.off >= len(r.b) {
		return 0, false
	}
	switch r.b[r.off] {
	case tagU32, tagI32:
		r.off++ // consume the tag; u32 reader below expects it gone
		if r.off+4 > len(r.b) {
			return 0, false
		}
		v := int64(int32(uint32(r.b[r.off]) | uint32(r.b[r.off+1])<<8 | uint32(r.b[r.off+2])<<16 | uint32(r.b[r.off+3])<<24))
		r.off += 4
		return v, true
	case tagU64, 0x09:
		r.off++
		if r.off+8 > len(r.b) {
			return 0, false
		}
		var v uint64
		for i := 0; i < 8; i++ {
			v |= uint64(r.b[r.off+i]) << (8 * i)
		}
		r.off += 8
		return int64(v), true
	}
	return 0, false
}

// dmlData is the server-owned bdDML blob (title-wide key/value data). Empty
// until a deployment configures it; kept persisted so it can be set out of band.
var dmlData = newDiskMap[[]byte]("dml")

// onDML handles bdDML (service 27). 27/2 getUserData takes no arguments and
// returns the title's DML blob.
func (l *lobbyConn) onDML(task byte, r *bdReader) []byte {
	if task != dmlGetUserData {
		return taskReply(task, 0, nil)
	}
	data, ok := dmlData.get("default")
	if !ok || len(data) == 0 || !framedReplies {
		return taskReply(task, 0, nil)
	}
	l.logf("dml getUserData -> %d bytes (framed, unverified)", len(data))
	return taskReply(task, 0, func(w *bdWriter) uint32 { w.blobv(data); return 1 })
}

// onEventLog handles bdEventLog (service 67). It accepts the telemetry payload,
// counts it for the dashboard and discards it: there is nothing to return, and
// an empty success is the right answer.
func (l *lobbyConn) onEventLog(task byte, r *bdReader) []byte {
	if task == eventLogRecord {
		eventLogTotal.Add(1)
		l.logf("eventlog record (%d bytes) -> accepted", len(r.b)-r.off)
	}
	return taskReply(task, 0, nil)
}
