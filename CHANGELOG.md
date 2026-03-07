# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project follows semantic versioning once stable releases begin.

## [Unreleased]

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
