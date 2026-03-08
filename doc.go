// Package busen provides a small typed-first in-process event bus for Go.
//
// Busen is designed around a few explicit goals:
//   - event payloads are plain Go values
//   - type-safe subscriptions are the primary API
//   - topics are optional local routing metadata
//   - context propagation is built into publish and handler execution
//   - asynchronous delivery uses bounded queues with explicit backpressure
//   - hooks expose runtime events without introducing a heavy framework layer
//   - middleware wraps local dispatch without turning the package into a framework
//   - optional metadata and observers support bridge/audit scenarios
//   - the package stays focused on simple in-process application use
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
// asynchronous subscriber with one worker, or within the same non-empty ordering
// key for async subscribers with multiple workers.
//
// Most applications start with [New], register handlers with [Subscribe],
// [SubscribeTopic], or [SubscribeTopics], and publish values with [Publish]. Use [Async],
// [Sequential], [WithParallelism], and [WithOverflow] when you need bounded
// asynchronous delivery, and [WithHooks] when you want to observe runtime
// errors, panics, dropped/rejected events, [UseObserver] for cross-cutting
// bridge observation, and [Shutdown] when you need explicit shutdown modes.
package busen
