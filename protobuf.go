package main

// Minimal protobuf wire-format encoder. The Diablo III online services carry their
// structured payloads as protobuf (schemas recovered from the game binary, see
// d3hack/capture/nso/protos/), so replies for leaderboards, mail and the like are
// built here field by field. Dependency-free, in the same spirit as the hand-written
// bdByteBuffer writer in services.go.
//
// Wire types: 0 varint, 1 fixed64, 2 length-delimited, 5 fixed32.

import "encoding/binary"

type pb struct {
	b []byte
}

func (p *pb) varint(v uint64) {
	for v >= 0x80 {
		p.b = append(p.b, byte(v)|0x80)
		v >>= 7
	}
	p.b = append(p.b, byte(v))
}

func (p *pb) tag(field int, wire byte) { p.varint(uint64(field)<<3 | uint64(wire)) }

// u64/u32/bool: varint field.
func (p *pb) u64(field int, v uint64) {
	if v == 0 {
		return // proto3-style: skip zero to keep messages small; required fields set explicitly
	}
	p.tag(field, 0)
	p.varint(v)
}

func (p *pb) u64always(field int, v uint64) {
	p.tag(field, 0)
	p.varint(v)
}

func (p *pb) boolean(field int, v bool) {
	if !v {
		return
	}
	p.tag(field, 0)
	p.varint(1)
}

// fixed64/fixed32.
func (p *pb) fixed64(field int, v uint64) {
	p.tag(field, 1)
	p.b = binary.LittleEndian.AppendUint64(p.b, v)
}

func (p *pb) fixed32(field int, v uint32) {
	p.tag(field, 5)
	p.b = binary.LittleEndian.AppendUint32(p.b, v)
}

// string/bytes/message: length-delimited.
func (p *pb) bytes(field int, v []byte) {
	p.tag(field, 2)
	p.varint(uint64(len(v)))
	p.b = append(p.b, v...)
}

func (p *pb) str(field int, s string) {
	if s == "" {
		return
	}
	p.bytes(field, []byte(s))
}

// message embeds a nested message built by fn.
func (p *pb) message(field int, fn func(*pb)) {
	var inner pb
	fn(&inner)
	p.bytes(field, inner.b)
}
