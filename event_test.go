// Tests for the virtio_input_event (de)serializer + the Event
// convenience accessors.
//
// SPDX-License-Identifier: BSD-3-Clause

package input

import (
	"encoding/binary"
	"errors"
	"testing"
)

func TestDecodeEvent_KeyDown(t *testing.T) {
	// KEY_A down: type=EvKey code=KeyA value=KeyValueDown.
	buf := make([]byte, EventWireSize)
	binary.LittleEndian.PutUint16(buf[0:2], EvKey)
	binary.LittleEndian.PutUint16(buf[2:4], KeyA)
	binary.LittleEndian.PutUint32(buf[4:8], KeyValueDown)
	ev, err := DecodeEvent(buf)
	if err != nil {
		t.Fatalf("DecodeEvent: %v", err)
	}
	if ev.Type != EvKey || ev.Code != KeyA || ev.Value != KeyValueDown {
		t.Errorf("DecodeEvent: got %+v", ev)
	}
	if !ev.IsKeyDown() {
		t.Error("IsKeyDown() = false")
	}
	if ev.IsKeyUp() || ev.IsKeyRepeat() {
		t.Error("IsKeyUp/IsKeyRepeat returned true on a key-down event")
	}
}

func TestDecodeEvent_KeyUp(t *testing.T) {
	buf := make([]byte, EventWireSize)
	binary.LittleEndian.PutUint16(buf[0:2], EvKey)
	binary.LittleEndian.PutUint16(buf[2:4], KeySpace)
	binary.LittleEndian.PutUint32(buf[4:8], KeyValueUp)
	ev, _ := DecodeEvent(buf)
	if !ev.IsKeyUp() {
		t.Error("IsKeyUp() = false on a KEY_SPACE up event")
	}
}

func TestDecodeEvent_KeyRepeat(t *testing.T) {
	buf := make([]byte, EventWireSize)
	binary.LittleEndian.PutUint16(buf[0:2], EvKey)
	binary.LittleEndian.PutUint16(buf[2:4], KeyZ)
	binary.LittleEndian.PutUint32(buf[4:8], KeyValueRepeat)
	ev, _ := DecodeEvent(buf)
	if !ev.IsKeyRepeat() {
		t.Error("IsKeyRepeat() = false on a KEY_Z repeat event")
	}
}

func TestDecodeEvent_RelMouseNegativeDelta(t *testing.T) {
	// REL_X with value=-3 (mouse moved left): the wire value is the
	// uint32 reinterpretation of the int32 -3 (0xFFFFFFFD).
	buf := make([]byte, EventWireSize)
	binary.LittleEndian.PutUint16(buf[0:2], EvRel)
	binary.LittleEndian.PutUint16(buf[2:4], RelX)
	neg3 := int32(-3)
	binary.LittleEndian.PutUint32(buf[4:8], uint32(neg3))
	ev, err := DecodeEvent(buf)
	if err != nil {
		t.Fatalf("DecodeEvent: %v", err)
	}
	if ev.SignedValue() != -3 {
		t.Errorf("SignedValue: got %d, want -3", ev.SignedValue())
	}
	if ev.Type != EvRel || ev.Code != RelX {
		t.Errorf("Type/Code: got (%d,%d), want (%d,%d)", ev.Type, ev.Code, EvRel, RelX)
	}
	if ev.IsKeyDown() {
		t.Error("IsKeyDown() returned true on an EvRel event")
	}
}

func TestDecodeEvent_SynReport(t *testing.T) {
	buf := make([]byte, EventWireSize)
	// EvSyn / SynReport / 0 is the canonical end-of-frame marker.
	ev, err := DecodeEvent(buf)
	if err != nil {
		t.Fatalf("DecodeEvent: %v", err)
	}
	if ev.Type != EvSyn || ev.Code != SynReport || ev.Value != 0 {
		t.Errorf("SYN_REPORT decode: got %+v", ev)
	}
}

func TestDecodeEvent_Short(t *testing.T) {
	if _, err := DecodeEvent(make([]byte, EventWireSize-1)); !errors.Is(err, ErrShortEvent) {
		t.Errorf("got %v, want ErrShortEvent", err)
	}
}

func TestEncodeEvent_RoundTrip(t *testing.T) {
	neg1 := int32(-1)
	in := Event{Type: EvRel, Code: RelWheel, Value: uint32(neg1)}
	buf := make([]byte, EventWireSize)
	if err := EncodeEvent(buf, in); err != nil {
		t.Fatalf("EncodeEvent: %v", err)
	}
	out, err := DecodeEvent(buf)
	if err != nil {
		t.Fatalf("DecodeEvent: %v", err)
	}
	if out != in {
		t.Errorf("round-trip: got %+v, want %+v", out, in)
	}
}

func TestEncodeEvent_Short(t *testing.T) {
	if err := EncodeEvent(make([]byte, EventWireSize-1), Event{}); !errors.Is(err, ErrShortEvent) {
		t.Errorf("got %v, want ErrShortEvent", err)
	}
}

func TestSentinelError(t *testing.T) {
	if got := ErrEventTimeout.Error(); got != string(ErrEventTimeout) {
		t.Errorf("Error(): got %q", got)
	}
}

func TestReadBufferBytes_NilGuard(t *testing.T) {
	if readBufferBytes(0, 8) != nil {
		t.Error("addr==0 should return nil")
	}
	if readBufferBytes(1234, 0) != nil {
		t.Error("length<=0 should return nil")
	}
}

func TestKeyCodeSpotChecks(t *testing.T) {
	// Spot-check that the kernel-constant values match what we expect;
	// drift would silently mis-decode key-codes.
	cases := []struct {
		name string
		got  uint16
		want uint16
	}{
		{"KeyA", KeyA, 30},
		{"KeyEscape", KeyEscape, 1},
		{"KeyEnter", KeyEnter, 28},
		{"KeySpace", KeySpace, 57},
		{"KeyUp", KeyUp, 103},
		{"KeyLeftCtrl", KeyLeftCtrl, 29},
		{"BtnLeft", BtnLeft, 0x110},
		{"RelX", RelX, 0},
		{"RelWheel", RelWheel, 8},
	}
	for _, c := range cases {
		if c.got != c.want {
			t.Errorf("%s: got %d, want %d", c.name, c.got, c.want)
		}
	}
}

func TestDecodeAll_HappyPath(t *testing.T) {
	// 3-event frame: REL_X delta + REL_Y delta + SYN_REPORT.
	buf := make([]byte, 3*EventWireSize)
	neg2 := int32(-2)
	_ = EncodeEvent(buf[0:8], Event{Type: EvRel, Code: RelX, Value: uint32(int32(5))})
	_ = EncodeEvent(buf[8:16], Event{Type: EvRel, Code: RelY, Value: uint32(neg2)})
	_ = EncodeEvent(buf[16:24], Event{Type: EvSyn, Code: SynReport, Value: 0})
	events, err := decodeAll(buf)
	if err != nil {
		t.Fatalf("decodeAll: %v", err)
	}
	if len(events) != 3 {
		t.Fatalf("len(events)=%d, want 3", len(events))
	}
	if events[0].SignedValue() != 5 || events[1].SignedValue() != -2 {
		t.Errorf("REL deltas: got %d / %d, want 5 / -2", events[0].SignedValue(), events[1].SignedValue())
	}
	if events[2].Type != EvSyn {
		t.Errorf("trailing SYN_REPORT: got type=%d", events[2].Type)
	}
}

func TestDecodeAll_Short(t *testing.T) {
	if _, err := decodeAll(make([]byte, 4)); !errors.Is(err, ErrShortEvent) {
		t.Errorf("got %v, want ErrShortEvent", err)
	}
}

func TestDecodeAll_PartialTrailingDropped(t *testing.T) {
	// One whole event + 4 trailing bytes (partial second event): the
	// trailing partial is silently dropped.
	buf := make([]byte, EventWireSize+4)
	_ = EncodeEvent(buf[0:8], Event{Type: EvSyn, Code: SynReport})
	events, err := decodeAll(buf)
	if err != nil {
		t.Fatalf("decodeAll: %v", err)
	}
	if len(events) != 1 {
		t.Errorf("len(events)=%d, want 1", len(events))
	}
}
