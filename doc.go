// Package busen provides a typed-first in-process event bus for Go.
//
// Busen is designed around a few explicit goals:
//   - event payloads are plain Go values
//   - type-safe subscriptions are the primary API
//   - topics are optional routing metadata
//   - context propagation is built into publish and handler execution
//   - asynchronous delivery uses bounded queues with explicit backpressure
//   - hooks expose runtime events without introducing a heavy framework layer
//
// Type-based subscriptions use exact Go types. A subscription registered for
// one type does not receive values of another type, even if they satisfy the
// same interface.
//
// Topic subscriptions use dot-separated topics. Wildcards are intentionally
// small in scope:
//   - "*" matches exactly one segment
//   - ">" matches one or more remaining segments and must be the last segment
//
// Ordering is never global. Busen only preserves FIFO delivery for a single
// asynchronous subscriber when it runs with one worker.
package busen
