// go-virtio/input — driver core: feature negotiation + init sequence +
// event-queue setup for the modern virtio-input device (Virtio 1.2
// §5.8).
//
// This driver targets the keyboard + relative-mouse baseline: it
// negotiates only VIRTIO_F_VERSION_1 (no input-specific feature bits
// are defined by the spec; future revisions may add some, at which
// point this mask is the place to extend) and the device exposes two
// virtqueues — an eventq (queue 0, device-to-guest events) and a
// statusq (queue 1, guest-to-device status / LED / force-feedback).
// The MVP only drives eventq; statusq is initialised but left empty.
//
// The driver pre-posts EventWireSize-aligned buffers on the eventq at
// bring-up so the device has somewhere to land guest input, and
// re-posts each buffer after a successful ReadEvent.
//
// SPDX-License-Identifier: BSD-3-Clause

package input

import (
	"github.com/go-virtio/common"
)

// PCIDeviceIDModernInput is the modern (Virtio 1.0+) PCI device ID for
// virtio-input. The device-type number is 18 (Virtio 1.2 §5.8), so
// DID = 0x1040 + 18 = 0x1052. go-virtio/common does not (yet) ship
// this constant — once it does, this local copy can fold into it.
const PCIDeviceIDModernInput uint16 = 0x1052

// EventQueueIdx / StatusQueueIdx are the two virtqueue indices for the
// virtio-input device (Virtio 1.2 §5.8.2).
const (
	EventQueueIdx  uint16 = 0
	StatusQueueIdx uint16 = 1
)

// EventQueueSize / StatusQueueSize are the desired ring sizes for the
// two queues. Clamped to the device's advertised maximum + rounded
// down to a power of two during setup.
const (
	EventQueueSize  uint16 = 64
	StatusQueueSize uint16 = 8
)

// EventBufferBytes is the size of one device-writable buffer on the
// eventq. We sized it to hold a few EventWireSize packets so the
// device can batch events into one descriptor when it likes — but the
// MVP read path reclaims after the first packet. Must be a multiple of
// EventWireSize so partial-packet truncation can't happen.
const EventBufferBytes = 64

// PollIterations is the default busy-poll budget ReadEvent(true)
// spends waiting for the device to populate an eventq buffer. A
// non-blocking ReadEvent(false) checks once and returns
// ErrEventNotReady on miss.
const PollIterations = 200000

// AcceptedFeatures is the feature mask the driver negotiates ON. For
// the MVP the only bit we ever accept is the non-negotiable
// VIRTIO_F_VERSION_1 (modern transport).
const AcceptedFeatures uint64 = common.FeatureVersion1

// AcceptFeatures returns the negotiated feature mask: the
// intersection of what the device offers and what we accept. The
// caller writes this back via DriverFeature.
//
// We require VIRTIO_F_VERSION_1 — if the device doesn't offer it, the
// device is legacy-only and we return ErrNotModernDevice.
func AcceptFeatures(deviceFeatures uint64) (uint64, error) {
	if deviceFeatures&common.FeatureVersion1 == 0 {
		return 0, ErrNotModernDevice
	}
	return deviceFeatures & AcceptedFeatures, nil
}

// VirtioInput wraps one initialised virtio-input device. The caller
// holds this for the lifetime of the device; the underlying virtqueue
// pages live as long as the supplied PageAllocator's lifetime
// contract.
type VirtioInput struct {
	// Cfg is the modern-transport handle (BARs + offsets + the
	// BARMemoryAccessor used for every register access).
	Cfg *common.ModernConfig

	// NegotiatedFeatures records what the driver-feature handshake
	// settled on. Exposed for diagnostic prints.
	NegotiatedFeatures uint64

	// Info bundles best-effort device-config information populated at
	// bring-up (device name, ...). All fields are zero-value if the
	// device does not expose a DeviceCfg region.
	Info DeviceInfo

	// transport is the underlying Transport — held so the data path
	// can route DMA-buffer allocations through the PageAllocator side.
	transport common.Transport

	// eventq / statusq are the two virtqueues set up by
	// OpenVirtioInput. statusq is reserved for future statusq-write
	// support; the MVP never touches it after init.
	eventq  *common.Virtqueue
	statusq *common.Virtqueue

	// pending buffers events 2..N from the most recent multi-packet
	// descriptor so ReadEvent can return one at a time without losing
	// the device's batching savings. Drained FIFO before the next
	// virtqueue poll.
	pending []Event
}

// OpenVirtioInput drives the full bring-up of one virtio-input
// device:
//
//  1. Verify the PCI device ID is 0x1052 (modern input).
//  2. InitModernConfig walks PCI caps + populates the BAR locators.
//  3. Reset → ACK → DRIVER status progression.
//  4. Read DeviceFeature, require VERSION_1, mask, write DriverFeature.
//  5. Set FEATURES_OK, verify it stuck.
//  6. Allocate + publish eventq (queue 0) + statusq (queue 1).
//  7. DRIVER_OK status.
//  8. Pre-post eventq buffers + notify the device.
//  9. Best-effort: probe device-config for the human-readable name.
//
// On success the device is in DRIVER_OK state, the eventq is
// pre-posted with EventBufferBytes-sized buffers, and the statusq is
// empty + ready (but not used by this MVP).
func OpenVirtioInput(t common.Transport) (*VirtioInput, error) {
	// Sanity-check this really is a modern virtio-input device.
	did, err := t.ReadConfig16(common.PCICfgDeviceID)
	if err != nil {
		return nil, err
	}
	if did != PCIDeviceIDModernInput {
		return nil, ErrInitWrongDeviceID
	}

	cfg, err := common.InitModernConfig(t)
	if err != nil {
		return nil, err
	}

	// Step 1: full reset (write 0 to DeviceStatus).
	if err := cfg.SetDeviceStatus(0); err != nil {
		return nil, err
	}
	if _, err := cfg.DeviceStatus(); err != nil {
		return nil, err
	}

	// Step 2: ACKNOWLEDGE.
	if err := cfg.SetDeviceStatus(common.StatusAcknowledge); err != nil {
		return nil, err
	}
	// Step 3: DRIVER.
	if err := cfg.SetDeviceStatus(common.StatusAcknowledge | common.StatusDriver); err != nil {
		return nil, err
	}

	// Step 4: read DeviceFeature, mask to our accepted set, write
	// DriverFeature.
	deviceFeats, err := cfg.DeviceFeatures64()
	if err != nil {
		return nil, err
	}
	if deviceFeats&common.FeatureVersion1 == 0 {
		return nil, ErrNotModernDevice
	}
	negotiated := deviceFeats & AcceptedFeatures
	if err := cfg.SetDriverFeatures64(negotiated); err != nil {
		return nil, err
	}

	// Step 5: FEATURES_OK + verify the device accepted our subset.
	if err := cfg.SetDeviceStatus(common.StatusAcknowledge | common.StatusDriver | common.StatusFeaturesOK); err != nil {
		return nil, err
	}
	status, err := cfg.DeviceStatus()
	if err != nil {
		return nil, err
	}
	if status&common.StatusFeaturesOK == 0 {
		return nil, ErrFeaturesNotOK
	}

	// Step 6: queue setup (eventq then statusq).
	eventq, err := setupQueue(cfg, t, EventQueueIdx, EventQueueSize)
	if err != nil {
		return nil, err
	}
	statusq, err := setupQueue(cfg, t, StatusQueueIdx, StatusQueueSize)
	if err != nil {
		return nil, err
	}

	// Step 7: DRIVER_OK.
	if err := cfg.SetDeviceStatus(common.StatusAcknowledge | common.StatusDriver | common.StatusFeaturesOK | common.StatusDriverOK); err != nil {
		return nil, err
	}

	v := &VirtioInput{
		Cfg:                cfg,
		NegotiatedFeatures: negotiated,
		transport:          t,
		eventq:             eventq,
		statusq:            statusq,
	}

	// Step 8: pre-post eventq buffers so the device has somewhere to
	// land guest input.
	if err := v.fillEventRing(); err != nil {
		return nil, err
	}
	// Notify the device that the eventq has buffers available.
	if err := cfg.NotifyQueue(EventQueueIdx, eventq.NotifyOff); err != nil {
		return nil, err
	}

	// Step 9: best-effort device-config probe for the device name.
	// Errors here are non-fatal — the data path doesn't need the name.
	if name, err := v.readDeviceName(); err == nil {
		v.Info.Name = name
	}

	return v, nil
}

// setupQueue performs the per-queue init: select, read max-size,
// write our size (= min(desired, max), rounded down to a power of
// two), allocate the Virtqueue, publish its descriptor/avail/used
// physical addresses, enable.
func setupQueue(cfg *common.ModernConfig, t common.Transport, queueIdx uint16, desiredSize uint16) (*common.Virtqueue, error) {
	if err := cfg.SelectQueue(queueIdx); err != nil {
		return nil, err
	}
	maxSize, err := cfg.QueueSize()
	if err != nil {
		return nil, err
	}
	if maxSize == 0 {
		return nil, ErrQueueNotAvailable
	}
	size := desiredSize
	if size > maxSize {
		size = maxSize
	}
	// Round size DOWN to a power of two; some QEMU versions report
	// non-power-of-two QueueSize on legacy queues.
	for size&(size-1) != 0 {
		size &= size - 1
	}
	if err := cfg.SetQueueSize(size); err != nil {
		return nil, err
	}
	notifyOff, err := cfg.QueueNotifyOff()
	if err != nil {
		return nil, err
	}
	q, err := common.NewVirtqueue(t, size, queueIdx, notifyOff)
	if err != nil {
		return nil, err
	}
	descAddr := q.BasePhys + uint64(q.Layout.DescTableOffset)
	availAddr := q.BasePhys + uint64(q.Layout.AvailRingOffset)
	usedAddr := q.BasePhys + uint64(q.Layout.UsedRingOffset)
	if err := cfg.SetQueueDesc(descAddr); err != nil {
		return nil, err
	}
	if err := cfg.SetQueueDriver(availAddr); err != nil {
		return nil, err
	}
	if err := cfg.SetQueueDevice(usedAddr); err != nil {
		return nil, err
	}
	if err := cfg.SetQueueEnable(1); err != nil {
		return nil, err
	}
	return q, nil
}

// EventQueue / StatusQueue expose the per-direction *common.Virtqueue
// handles. Read-only accessors so callers can inspect ring state for
// diagnostic dumps; the fields themselves stay unexported.
func (v *VirtioInput) EventQueue() *common.Virtqueue { return v.eventq }

// StatusQueue returns the status (guest-to-device) virtqueue handle.
func (v *VirtioInput) StatusQueue() *common.Virtqueue { return v.statusq }

// fillEventRing posts one device-writable EventBufferBytes-sized
// buffer per eventq slot so the device has somewhere to land guest
// input.
func (v *VirtioInput) fillEventRing() error {
	for i := uint16(0); i < v.eventq.Layout.Size; i++ {
		phys, mem, err := v.transport.AllocatePages(1)
		if err != nil {
			return err
		}
		if phys == 0 {
			return common.ErrAllocReturnedZero
		}
		if len(mem) < int(EventBufferBytes) {
			return ErrBufferTooSmall
		}
		// writable=true ⇒ VIRTQ_DESC_F_WRITE set.
		addr := uintptr(phys) // identity-mapped on supported hosts
		if _, err := v.eventq.AddBuffer(addr, phys, uint32(EventBufferBytes), true); err != nil {
			return err
		}
	}
	return nil
}

// Sentinel errors for the virtio-input path. All exported so callers
// can branch + format them.
var (
	ErrNotModernDevice   = commonInputError("go-virtio/input: device doesn't offer VIRTIO_F_VERSION_1 (legacy-only)")
	ErrFeaturesNotOK     = commonInputError("go-virtio/input: FEATURES_OK status bit didn't stick after DriverFeature write")
	ErrInitWrongDeviceID = commonInputError("go-virtio/input: PCI device ID is not 0x1052 (modern input device)")
	ErrQueueNotAvailable = commonInputError("go-virtio/input: device reports QueueSize=0 for a required queue")
	ErrEventTimeout      = commonInputError("go-virtio/input: event poll timeout (no event received within budget)")
	ErrEventNotReady     = commonInputError("go-virtio/input: non-blocking ReadEvent: no event available")
	ErrBufferTooSmall    = commonInputError("go-virtio/input: PageAllocator returned a chunk smaller than EventBufferBytes")
	ErrShortEvent        = commonInputError("go-virtio/input: event buffer shorter than EventWireSize (8 bytes)")
)

// commonInputError is the package's tiny sentinel-error type — same
// pattern as go-virtio/common.commonError and go-virtio/net.commonNetError.
type commonInputError string

// Error implements the `error` interface.
func (e commonInputError) Error() string { return string(e) }
