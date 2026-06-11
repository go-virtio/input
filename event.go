// go-virtio/input — event wire format (Virtio 1.2 §5.8.6).
//
// The virtio-input device delivers events using Linux's evdev
// `input_event` shape, packed into one 8-byte descriptor entry:
//
//	struct virtio_input_event {
//	    le16 type;
//	    le16 code;
//	    le32 value;
//	};
//
// `type` is one of the EV_* class constants (EvKey, EvRel, EvAbs,
// EvSyn, ...). `code` is the per-class subcode (KeyA, RelX, AbsX, ...).
// `value` is the event's payload — for EvKey it is one of the
// KeyValue* constants (Up=0, Down=1, Repeat=2); for EvRel it is the
// signed delta along the named axis (encoded as int32 — see SignedValue).
//
// This file owns the on-wire constants and the (de)serializers; the
// virtqueue-level data path lives in read.go.
//
// SPDX-License-Identifier: BSD-3-Clause

package input

import (
	"encoding/binary"
)

// EventWireSize is the size in bytes of one virtio_input_event packet
// on the wire. Virtio 1.2 §5.8.6 fixes the layout at 2 + 2 + 4 = 8 bytes
// little-endian.
const EventWireSize = 8

// Event-class (type) constants — the high-level classification of an
// event, identical to Linux's EV_* values from input-event-codes.h.
// Only the classes this MVP recognises are enumerated; the device may
// emit other classes (EvLED, EvSnd, EvFF, ...) which the driver
// surfaces unchanged so callers can branch on them if needed.
const (
	// EvSyn (0x00) — synchronization marker; ends an event group.
	// Code SynReport (0) is the most common.
	EvSyn uint16 = 0x00
	// EvKey (0x01) — key / button press / release / auto-repeat.
	EvKey uint16 = 0x01
	// EvRel (0x02) — relative axis (mouse X / Y / wheel).
	EvRel uint16 = 0x02
	// EvAbs (0x03) — absolute axis (tablet / touchscreen). Out of MVP
	// scope but the constant is exposed so callers can branch on it.
	EvAbs uint16 = 0x03
	// EvMsc (0x04) — miscellaneous (scancode passthrough, ...).
	EvMsc uint16 = 0x04
	// EvLED (0x11) — LED state (CapsLock / NumLock / ScrollLock). Set
	// via statusq; this driver does not write to statusq in the MVP.
	EvLED uint16 = 0x11
)

// SynReport (0) is the code paired with EvSyn that signals the end of
// an event group (e.g. a complete mouse move = REL_X delta + REL_Y
// delta + SYN_REPORT). Callers that batch events into "frames" use
// this as the group delimiter.
const SynReport uint16 = 0x00

// KeyValueUp / KeyValueDown / KeyValueRepeat are the three values an
// EvKey event's `value` field can take, matching Linux evdev semantics.
const (
	KeyValueUp     uint32 = 0
	KeyValueDown   uint32 = 1
	KeyValueRepeat uint32 = 2
)

// Event is the decoded form of one virtio_input_event packet. Type and
// Code are the on-wire uint16 fields verbatim; Value is the raw uint32
// from the wire — callers that need a signed delta use SignedValue.
type Event struct {
	// Type is the EV_* class (see EvKey, EvRel, EvSyn, ...).
	Type uint16
	// Code is the per-class subcode (KeyA, RelX, BtnLeft, SynReport,
	// ...).
	Code uint16
	// Value is the raw 32-bit payload. For EvKey it is one of
	// KeyValueUp / KeyValueDown / KeyValueRepeat. For EvRel it is the
	// signed delta along the named axis — re-interpret via SignedValue.
	Value uint32
}

// SignedValue re-interprets the raw uint32 Value as a signed int32 —
// the natural shape for EvRel deltas (negative on REL_X = mouse left,
// negative on REL_WHEEL = wheel down, ...).
func (e Event) SignedValue() int32 { return int32(e.Value) }

// IsKeyDown reports whether the event is a key-press transition
// (EvKey with KeyValueDown). Convenience for callers that don't want
// to compare against the two constants by hand.
func (e Event) IsKeyDown() bool { return e.Type == EvKey && e.Value == KeyValueDown }

// IsKeyUp reports whether the event is a key-release transition
// (EvKey with KeyValueUp).
func (e Event) IsKeyUp() bool { return e.Type == EvKey && e.Value == KeyValueUp }

// IsKeyRepeat reports whether the event is an auto-repeat
// (EvKey with KeyValueRepeat).
func (e Event) IsKeyRepeat() bool { return e.Type == EvKey && e.Value == KeyValueRepeat }

// DecodeEvent parses one virtio_input_event from the first
// EventWireSize bytes of `buf`. Returns ErrShortEvent if the slice is
// shorter than EventWireSize.
//
// All fields are little-endian on the wire (Virtio 1.2 §1.4): every
// Go-supported architecture is also natively little-endian so the
// memory access pattern is straight loads on the host side; we route
// through encoding/binary anyway so the package stays endian-portable
// to non-LE hosts if Go ever ships one.
func DecodeEvent(buf []byte) (Event, error) {
	if len(buf) < EventWireSize {
		return Event{}, ErrShortEvent
	}
	return Event{
		Type:  binary.LittleEndian.Uint16(buf[0:2]),
		Code:  binary.LittleEndian.Uint16(buf[2:4]),
		Value: binary.LittleEndian.Uint32(buf[4:8]),
	}, nil
}

// EncodeEvent writes `ev` into the first EventWireSize bytes of `buf`
// in the virtio_input_event wire format. Returns ErrShortEvent if the
// slice is shorter than EventWireSize.
//
// The driver itself never produces events (the device is the source);
// EncodeEvent exists so tests can inject synthetic events through the
// fake transport's deliver path, and so downstream code that bridges
// guest input through a vsock or pipe can stay symmetric with
// DecodeEvent.
func EncodeEvent(buf []byte, ev Event) error {
	if len(buf) < EventWireSize {
		return ErrShortEvent
	}
	binary.LittleEndian.PutUint16(buf[0:2], ev.Type)
	binary.LittleEndian.PutUint16(buf[2:4], ev.Code)
	binary.LittleEndian.PutUint32(buf[4:8], ev.Value)
	return nil
}
