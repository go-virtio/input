// go-virtio/input — Linux input-event-codes subset.
//
// The constants in this file are a hand-picked slice of Linux's
// `include/uapi/linux/input-event-codes.h`, enough to drive the
// cloud-boot DOOM port and any other "keyboard + relative mouse"
// guest:
//
//   - Alphanumeric keys (KeyA..KeyZ, Key0..Key9).
//   - Function keys (KeyF1..KeyF12).
//   - Arrow keys + the navigation block (Home / End / PgUp / PgDn /
//     Insert / Delete).
//   - Modifiers (LeftCtrl / LeftShift / LeftAlt / LeftMeta + the right-
//     hand counterparts).
//   - Control keys (Escape / Enter / Space / Tab / Backspace /
//     CapsLock).
//   - Relative-axis codes (RelX / RelY / RelWheel).
//   - Mouse buttons (BtnLeft / BtnRight / BtnMiddle / BtnSide /
//     BtnExtra).
//
// Full coverage of Linux's table (gamepad axes, brightness, multimedia
// hotkeys, ...) is deliberately not in scope — adding more constants
// later is a one-liner per code and breaks nothing.
//
// The values are stable wire identifiers (the kernel ABI doesn't
// re-number them); the integers below are reproduced from
// upstream Linux for cross-checking.
//
// SPDX-License-Identifier: BSD-3-Clause

package input

// Letters — KEY_A..KEY_Z (kernel codes 30..54).
const (
	KeyA uint16 = 30
	KeyB uint16 = 48
	KeyC uint16 = 46
	KeyD uint16 = 32
	KeyE uint16 = 18
	KeyF uint16 = 33
	KeyG uint16 = 34
	KeyH uint16 = 35
	KeyI uint16 = 23
	KeyJ uint16 = 36
	KeyK uint16 = 37
	KeyL uint16 = 38
	KeyM uint16 = 50
	KeyN uint16 = 49
	KeyO uint16 = 24
	KeyP uint16 = 25
	KeyQ uint16 = 16
	KeyR uint16 = 19
	KeyS uint16 = 31
	KeyT uint16 = 20
	KeyU uint16 = 22
	KeyV uint16 = 47
	KeyW uint16 = 17
	KeyX uint16 = 45
	KeyY uint16 = 21
	KeyZ uint16 = 44
)

// Digits — KEY_0..KEY_9 (codes 2..11).
const (
	Key1 uint16 = 2
	Key2 uint16 = 3
	Key3 uint16 = 4
	Key4 uint16 = 5
	Key5 uint16 = 6
	Key6 uint16 = 7
	Key7 uint16 = 8
	Key8 uint16 = 9
	Key9 uint16 = 10
	Key0 uint16 = 11
)

// Function keys F1..F12 (codes 59..68 + 87..88).
const (
	KeyF1  uint16 = 59
	KeyF2  uint16 = 60
	KeyF3  uint16 = 61
	KeyF4  uint16 = 62
	KeyF5  uint16 = 63
	KeyF6  uint16 = 64
	KeyF7  uint16 = 65
	KeyF8  uint16 = 66
	KeyF9  uint16 = 67
	KeyF10 uint16 = 68
	KeyF11 uint16 = 87
	KeyF12 uint16 = 88
)

// Control + whitespace + editing keys.
const (
	KeyEscape    uint16 = 1
	KeyBackspace uint16 = 14
	KeyTab       uint16 = 15
	KeyEnter     uint16 = 28
	KeySpace     uint16 = 57
	KeyCapsLock  uint16 = 58
)

// Arrows + navigation block.
const (
	KeyUp       uint16 = 103
	KeyLeft     uint16 = 105
	KeyRight    uint16 = 106
	KeyDown     uint16 = 108
	KeyHome     uint16 = 102
	KeyEnd      uint16 = 107
	KeyPageUp   uint16 = 104
	KeyPageDown uint16 = 109
	KeyInsert   uint16 = 110
	KeyDelete   uint16 = 111
)

// Modifier keys — left + right pairs (Ctrl / Shift / Alt / Meta).
const (
	KeyLeftCtrl   uint16 = 29
	KeyLeftShift  uint16 = 42
	KeyRightShift uint16 = 54
	KeyLeftAlt    uint16 = 56
	KeyRightCtrl  uint16 = 97
	KeyRightAlt   uint16 = 100
	KeyLeftMeta   uint16 = 125
	KeyRightMeta  uint16 = 126
)

// Relative-axis codes — paired with EvRel events.
//
// RelWheel carries one wheel notch per event (positive = up, negative =
// down) on the standard mouse-wheel device.
const (
	RelX        uint16 = 0x00
	RelY        uint16 = 0x01
	RelZ        uint16 = 0x02
	RelHWheel   uint16 = 0x06
	RelWheel    uint16 = 0x08
	RelMisc     uint16 = 0x09
	RelMaxValue uint16 = 0x0F
)

// Mouse-button codes — paired with EvKey events (buttons are
// technically a sub-range of the key codes). The wire codes match
// Linux's BTN_LEFT / BTN_RIGHT / BTN_MIDDLE etc.
const (
	BtnLeft   uint16 = 0x110
	BtnRight  uint16 = 0x111
	BtnMiddle uint16 = 0x112
	BtnSide   uint16 = 0x113
	BtnExtra  uint16 = 0x114
)
