// go-virtio/input — device-config layout (Virtio 1.2 §5.8.4).
//
// The virtio-input device exposes a `struct virtio_input_config` in
// device-config space:
//
//	struct virtio_input_config {
//	    u8 select;       // sub-register selector (CfgSel*)
//	    u8 subsel;       // sub-selector within `select`
//	    u8 size;         // device-written: number of valid bytes in u.*
//	    u8 reserved[5];
//	    union {
//	        char  string[128];    // device name / serial / device-ids
//	        u8    bitmap[128];    // EV_ / KEY_ / ABS_ supported masks
//	        struct virtio_input_absinfo abs;
//	        struct virtio_input_devids  ids;
//	    } u;
//	};
//
// The host writes (select, subsel); the device populates `size` and
// `u.*` with the requested information. The driver MAY enumerate the
// device's capabilities by walking `select` ∈ {CfgIDName, CfgIDSerial,
// CfgIDDevIDs, CfgPropBits, CfgEVBits, CfgAbsInfo} and, for class
// bitmaps, `subsel` ∈ {EvKey, EvRel, EvAbs, ...}.
//
// The MVP driver only reads the device name (CfgIDName). The rest of
// the constants are exposed so future code (force-feedback, abs-axis
// support) can grow into them without touching go-virtio/common.
//
// SPDX-License-Identifier: BSD-3-Clause

package input

// Device-config field offsets — byte offsets inside the
// virtio_input_config struct (Virtio 1.2 §5.8.4).
const (
	// CfgFieldSelect is the host-written sub-register selector.
	CfgFieldSelect uint32 = 0x00
	// CfgFieldSubsel is the host-written sub-selector qualifier.
	CfgFieldSubsel uint32 = 0x01
	// CfgFieldSize is the device-written length of the populated
	// portion of the union (in bytes).
	CfgFieldSize uint32 = 0x02
	// CfgFieldUnion is the byte offset of the start of the union
	// (`string` / `bitmap` / `abs` / `ids`).
	CfgFieldUnion uint32 = 0x08
	// CfgUnionSize is the size in bytes of the union. The string +
	// bitmap variants are 128 bytes; abs is 20 bytes; ids is 8 bytes.
	// 128 is the upper bound on what the device may populate.
	CfgUnionSize uint32 = 128
	// CfgTotalSize is the total size of virtio_input_config in bytes.
	CfgTotalSize uint32 = CfgFieldUnion + CfgUnionSize
)

// `select` values — chosen by the host to ask the device what kind of
// information to write into the union (Virtio 1.2 §5.8.4 table).
const (
	// CfgSelUnset is the initial / "no selection" state — the device
	// returns size=0.
	CfgSelUnset uint8 = 0x00
	// CfgSelIDName asks for a free-form ASCII device name (e.g.
	// "QEMU Virtio Keyboard") in u.string.
	CfgSelIDName uint8 = 0x01
	// CfgSelIDSerial asks for a free-form ASCII serial in u.string.
	CfgSelIDSerial uint8 = 0x02
	// CfgSelIDDevIDs asks for a virtio_input_devids struct in u.ids
	// (bustype, vendor, product, version — Linux input ABI).
	CfgSelIDDevIDs uint8 = 0x03
	// CfgSelPropBits asks for the INPUT_PROP_* property bitmap in
	// u.bitmap.
	CfgSelPropBits uint8 = 0x10
	// CfgSelEVBits asks "which event-class bits does the device emit?"
	// — subsel is then the EV_* class id (EvKey, EvRel, ...).
	CfgSelEVBits uint8 = 0x11
	// CfgSelAbsInfo asks for the per-axis absinfo (min/max/fuzz/flat/
	// resolution). subsel is the ABS_* axis id.
	CfgSelAbsInfo uint8 = 0x12
)

// DeviceInfo bundles the device-config information OpenVirtioInput
// extracts during bring-up. All fields are best-effort: a device that
// returns size=0 for a given select leaves the corresponding field at
// its zero value (empty string).
type DeviceInfo struct {
	// Name is the human-readable device name reported by CfgSelIDName.
	// May be empty if the device does not implement the selector.
	Name string
}

// readDeviceName drives the (select, subsel) handshake to pull the
// device's free-form name out of device-config space.
//
// Steps:
//
//  1. Write CfgSelIDName into the `select` byte.
//  2. Write 0 into the `subsel` byte (unused for the IDName variant).
//  3. Read `size` to discover how many valid bytes the device wrote
//     into the union region.
//  4. Read `size` bytes from u.string, trim a trailing NUL if present.
//
// Any transport-level error short-circuits with (zero, err). A device
// that doesn't expose a DeviceCfg region (HasDeviceCfg=false) returns
// ("", nil) — the name is best-effort, the absence is not an error.
//
// Lives here rather than in input.go to keep the device-config probe
// self-contained and easy to unit-test against the fake transport.
func (v *VirtioInput) readDeviceName() (string, error) {
	if !v.Cfg.HasDeviceCfg() {
		return "", nil
	}
	// Helper: write one byte into device-config.
	writeByte := func(off uint32, val uint8) error {
		return v.Cfg.BAR.Write8(v.Cfg.DeviceCfgBAR, v.Cfg.DeviceCfgOffset+uint64(off), val)
	}
	if err := writeByte(CfgFieldSelect, CfgSelIDName); err != nil {
		return "", err
	}
	if err := writeByte(CfgFieldSubsel, 0); err != nil {
		return "", err
	}
	size, err := v.Cfg.DeviceCfgRead8(CfgFieldSize)
	if err != nil {
		return "", err
	}
	if size == 0 {
		return "", nil
	}
	if uint32(size) > CfgUnionSize {
		size = uint8(CfgUnionSize)
	}
	buf := make([]byte, size)
	for i := uint8(0); i < size; i++ {
		b, err := v.Cfg.DeviceCfgRead8(CfgFieldUnion + uint32(i))
		if err != nil {
			return "", err
		}
		buf[i] = b
	}
	// Trim trailing NUL terminator if the device included one.
	for len(buf) > 0 && buf[len(buf)-1] == 0 {
		buf = buf[:len(buf)-1]
	}
	return string(buf), nil
}
