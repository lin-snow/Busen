# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project follows semantic versioning once stable releases begin.

## [Unreleased]

### Added

- Added optional structured envelope metadata support via
  `WithMetadataBuilder(...)` and per-publish `WithMetadata(...)`.
- Added `Observation` / `Observer` and `UseObserver(...)` for bridge-style
  cross-cutting observation of accepted deliveries.
- Added observer filter options: `ObserveType[T]()`, `ObserveTopic(...)`,
  `ObserveMetadata(...)`, and `ObserveMatch(...)`.
- Added explicit shutdown modes with structured outcomes:
  `ShutdownDrain`, `ShutdownBestEffort`, `ShutdownAbort`, and
  `ShutdownResult`.
- Added `Hooks.OnEventRejected` and `RejectedEvent` for async fail-fast
  rejection observation.
- Added extra drop/reject diagnostics in hook payloads, including
  `SubscriberID`, queue length/capacity, and mailbox index.
- Added examples covering metadata builder usage, observer usage, and shutdown
  modes.
- Added benchmarks for metadata and observer enabled/disabled overhead.

### Changed

- Updated `Close(ctx)` implementation to delegate to
  `Shutdown(ctx, ShutdownDrain)` while preserving close semantics.
- Renamed observer-facing API to shorter names:
  `DispatchObservation` -> `Observation`,
  `DispatchObserver` -> `Observer`,
  `UseDispatchObserver(...)` -> `UseObserver(...)`.
- Refreshed `README.md` structure with expanded quick overview, consolidated
  "核心优势与能力", observer terminology, and updated benchmark reference values.

### Fixed

- Clarified observer-related comments and validation error wording for naming
  consistency.

## [v0.2.0] - 2026-03-08

### Added

- Added `SubscribeTopics[T]` for registering one typed handler against multiple
  topic patterns with OR matching semantics.
- Added an example and README guidance for multi-topic typed subscriptions.

### Fixed

- Added regression tests covering overlapping topic patterns, invalid pattern
  handling, filter composition, unified unsubscribe behavior, and hook counting
  semantics for `SubscribeTopics[T]`.

## [v0.1.0] - 2026-03-08

### Added

- Added `PublishDone.DeliveredSubscribers` to distinguish routing matches from
  deliveries that actually started handler execution or async enqueue.
- Added `Hooks.OnHookPanic` and `HookPanic` so hook panics can be observed
  without breaking the main publish or handler flow.
- Added `ErrCloseIncomplete` to make `Close(ctx)` timeout outcomes explicit.
- Added example-based documentation for basic publish/subscribe, topic routing,
  async delivery, keyed ordering, middleware, and hooks.
- Added API selection guidance to `README.md`.

### Changed

- Cached per-type subscription snapshots so `Publish` no longer rebuilds the
  subscriber slice on every call.
- Cached middleware chain assembly and per-subscription wrapped dispatch to
  reduce repeated work on the hot path.
- Expanded exported API documentation comments for options, events, dispatch,
  and hook payloads.
- Clarified `Close(ctx)` semantics in documentation: timeout means draining did
  not finish in time, not that user handlers were forcibly terminated.

### Fixed

- Fixed publish hook accounting so matched subscribers and delivered
  subscribers are reported separately.
- Added tests covering hook panic recovery, matched vs delivered accounting,
  and close timeout behavior.
