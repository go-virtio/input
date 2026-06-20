<p align="center"><img src="https://raw.githubusercontent.com/go-virtio/brand/main/social/go-virtio-input.png" alt="go-virtio/input" width="720"></p>

# go-virtio/input

Pure-Go virtio-input driver targeting the `go-virtio/common` transport
interfaces. Implements the modern-transport (Virtio 1.0+) init sequence
and the device-to-guest event read path for the standard PCI-bound
virtio-input device (VID 0x1AF4, DID 0x1052).

This package targets the keyboard + relative-pointer baseline (Virtio
1.2 §5.8): it negotiates only `VIRTIO_F_VERSION_1` and the device
exposes two virtqueues — `eventq` (queue 0, device-to-guest events) and
`statusq` (queue 1, guest-to-device status, e.g. LED + force-feedback,
NOT driven by this MVP). The event wire format mirrors Linux's
`input_event` structure (type, code, value), and the key-code constants
are a hand-picked subset of Linux's `input-event-codes.h` — enough for
keyboard input plus relative mouse events (the DOOM port's input
budget).

The driver pre-posts buffers on the eventq at bring-up so the device
has somewhere to land guest input, and re-posts the same buffer after
each successful `ReadEvent` call. Every transport-level operation is
routed through `go-virtio/common`'s `Transport` interface, so any
implementation of that interface (UEFI's `EFI_PCI_IO_PROTOCOL`,
bare-metal MMIO, virtio-mmio adapter) drives the same driver code.

## Quick start

```go
import (
    virtioinput "github.com/go-virtio/input"
)

// transport is any value that implements go-virtio/common.Transport.
vi, err := virtioinput.OpenVirtioInput(transport)
if err != nil {
    return err
}

// Blocking read (busy-polls until an event is consumed).
ev, err := vi.ReadEvent(true)
if err != nil {
    return err
}
switch ev.Type {
case virtioinput.EvKey:
    // ev.Code is e.g. KeyA, KeySpace, BtnLeft. ev.Value is 1 (down), 0
    // (up), or 2 (auto-repeat).
case virtioinput.EvRel:
    // ev.Code is RelX / RelY / RelWheel; ev.Value is the signed delta.
case virtioinput.EvSyn:
    // End-of-event-group marker (SYN_REPORT / SYN_DROPPED / ...).
}

// Non-blocking read (returns ErrEventNotReady immediately if the
// eventq is empty).
ev, err = vi.ReadEvent(false)
```

`OpenVirtioInput` leaves the device in DRIVER_OK state with the eventq
pre-posted with one-page buffers.

## Scope

In scope:

  - Keyboard scancodes (alphanumerics, arrows, ESC, enter, space, common
    modifiers — Linux `KEY_*` subset).
  - Relative mouse events (`REL_X`, `REL_Y`, `REL_WHEEL`).
  - Mouse buttons (`BTN_LEFT`, `BTN_RIGHT`, `BTN_MIDDLE`).
  - Blocking + non-blocking event read.

Out of scope (deliberately, for the cloud-boot DOOM sprint):

  - Force feedback (statusq write side).
  - Touchscreen multitouch.
  - Tablet absolute coordinates.
  - Joystick / gamepad axes.

## Sibling packages

  - [`github.com/go-virtio/common`](https://github.com/go-virtio/common)
    — transport-agnostic infrastructure (PCI cap walker, modern config
    layout, split-virtqueue impl, transport interfaces).
  - [`github.com/go-virtio/net`](https://github.com/go-virtio/net) —
    pure-Go virtio-net driver.
  - [`github.com/go-virtio/console`](https://github.com/go-virtio/console)
    — pure-Go virtio-console driver (the closest single-direction-data
    sibling).
  - [`github.com/go-virtio/rng`](https://github.com/go-virtio/rng) —
    pure-Go virtio-rng driver.

## License

BSD-3-Clause. See [LICENSE](LICENSE).
