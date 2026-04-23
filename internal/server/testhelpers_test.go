package server

import "sync/atomic"

// callCounter is a tiny atomic hit counter shared by the region and landmark
// counting source doubles. Exposed as a struct rather than a bare atomic.Int64
// so future helpers (elapsed-time, last-key capture) can extend it without
// touching every embedder.
type callCounter struct {
	calls atomic.Int64
}

// hit records one delegated call.
func (c *callCounter) hit() { c.calls.Add(1) }

// count returns the number of delegated calls observed so far.
func (c *callCounter) count() int64 { return c.calls.Load() }
