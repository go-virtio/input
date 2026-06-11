// go-virtio/input — eventq read path.
//
// The virtio-input device delivers events on the eventq (queue 0) as a
// stream of virtio_input_event packets. Each pre-posted descriptor
// will be filled by the device with 1..N packets (a typical mouse
// move is 3 packets: REL_X delta + REL_Y delta + SYN_REPORT) and
// returned via the used ring with the populated byte count.
//
// The MVP read path takes the simpler "one event per ReadEvent"
// shape: ReadEvent pulls one descriptor, decodes the FIRST packet, and
// re-posts the buffer. Events 2..N from a multi-packet descriptor are
// not lost — they are buffered in v.pending and consumed by the next
// ReadEvent before the next virtqueue poll. This keeps the API simple
// (one event return) while preserving the device's batching savings.
//
// SPDX-License-Identifier: BSD-3-Clause

package input

// ReadEvent consumes one virtio_input_event from the eventq.
//
// If blocking is true, the call busy-polls up to PollIterations times
// waiting for the device to fill a descriptor, and returns
// ErrEventTimeout if the budget is exhausted.
//
// If blocking is false, the call checks the eventq exactly once and
// returns ErrEventNotReady immediately if no descriptor is populated.
//
// On success the returned *Event is freshly allocated — safe to retain
// after this call returns (and after the descriptor it came from is
// reclaimed and re-posted).
//
// The implementation drains a descriptor's full event-packet count
// into v.pending so a single virtqueue round-trip can produce multiple
// events for callers that batch input frames (mouse REL_X + REL_Y +
// SYN_REPORT is the canonical example).
func (v *VirtioInput) ReadEvent(blocking bool) (*Event, error) {
	// 1. If we already have a buffered event from a previous
	//    multi-packet descriptor, return it immediately.
	if len(v.pending) > 0 {
		ev := v.pending[0]
		v.pending = v.pending[1:]
		return &ev, nil
	}
	// 2. Otherwise poll the eventq.
	budget := PollIterations
	if !blocking {
		budget = 1
	}
	for spin := 0; spin < budget; spin++ {
		descIdx, length, ok := v.eventq.PollUsed()
		if !ok {
			continue
		}
		buf := v.eventq.Buffers[descIdx]
		raw := readBufferBytes(buf.Addr, int(length))
		events, decodeErr := decodeAll(raw)
		_ = v.eventq.Reclaim(descIdx)
		// Re-post the same buffer (it's still allocated) so the device
		// has somewhere to land the next batch.
		if _, err := v.eventq.AddBuffer(buf.Addr, buf.Phys, buf.Len, true); err != nil {
			// Re-post failed; we're degraded but the captured events
			// are still good to return.
			_ = err
		}
		if err := v.Cfg.NotifyQueue(EventQueueIdx, v.eventq.NotifyOff); err != nil {
			_ = err
		}
		if decodeErr != nil {
			return nil, decodeErr
		}
		// Stash trailing events for the next ReadEvent and return the
		// first one. decodeAll guarantees len(events) >= 1 on success.
		if len(events) > 1 {
			v.pending = append(v.pending, events[1:]...)
		}
		out := events[0]
		return &out, nil
	}
	if !blocking {
		return nil, ErrEventNotReady
	}
	return nil, ErrEventTimeout
}

// decodeAll walks `raw` in EventWireSize-sized chunks and returns the
// decoded events. A trailing partial chunk is ignored (the device is
// not supposed to produce one — Virtio 1.2 §5.8.6 mandates whole
// events — but tolerating it keeps the driver robust to malformed
// devices).
//
// Returns ErrShortEvent only if `raw` is shorter than ONE event; if
// any whole event was decoded the trailing remainder (if any) is
// silently dropped.
func decodeAll(raw []byte) ([]Event, error) {
	if len(raw) < EventWireSize {
		return nil, ErrShortEvent
	}
	n := len(raw) / EventWireSize
	out := make([]Event, 0, n)
	for i := 0; i < n; i++ {
		ev, err := DecodeEvent(raw[i*EventWireSize : (i+1)*EventWireSize])
		if err != nil {
			return nil, err
		}
		out = append(out, ev)
	}
	return out, nil
}
