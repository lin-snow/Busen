package busen

import (
	"reflect"
)

// Hooks observes publish and handler lifecycle events.
//
// Hooks are intentionally thin. They are not a full middleware pipeline and do
// not change delivery semantics. They exist to surface important runtime events
// such as async failures, panics, and dropped events.
type Hooks struct {
	OnPublishStart func(PublishStart)
	OnPublishDone  func(PublishDone)
	OnHandlerError func(HandlerError)
	OnHandlerPanic func(HandlerPanic)
	OnEventDropped func(DroppedEvent)
}

// PublishStart describes the beginning of a publish operation.
type PublishStart struct {
	EventType reflect.Type
	Topic     string
	Key       string
	Headers   map[string]string
}

// PublishDone describes the end of a publish operation.
type PublishDone struct {
	EventType          reflect.Type
	Topic              string
	Key                string
	Headers            map[string]string
	MatchedSubscribers int
	Err                error
}

// HandlerError describes a handler error.
type HandlerError struct {
	EventType reflect.Type
	Topic     string
	Key       string
	Async     bool
	Err       error
}

// HandlerPanic describes a recovered handler panic.
type HandlerPanic struct {
	EventType reflect.Type
	Topic     string
	Key       string
	Async     bool
	Value     any
}

// DroppedEvent describes a dropped event caused by backpressure.
type DroppedEvent struct {
	EventType reflect.Type
	Topic     string
	Key       string
	Async     bool
	Policy    OverflowPolicy
	Reason    error
}

func mergeHooks(dst *Hooks, src Hooks) {
	if dst == nil {
		return
	}

	dst.OnPublishStart = chainPublishStart(dst.OnPublishStart, src.OnPublishStart)
	dst.OnPublishDone = chainPublishDone(dst.OnPublishDone, src.OnPublishDone)
	dst.OnHandlerError = chainHandlerError(dst.OnHandlerError, src.OnHandlerError)
	dst.OnHandlerPanic = chainHandlerPanic(dst.OnHandlerPanic, src.OnHandlerPanic)
	dst.OnEventDropped = chainDroppedEvent(dst.OnEventDropped, src.OnEventDropped)
}

func chainPublishStart(a, b func(PublishStart)) func(PublishStart) {
	switch {
	case a == nil:
		return b
	case b == nil:
		return a
	default:
		return func(info PublishStart) {
			safeCall(func() { a(info) })
			safeCall(func() { b(info) })
		}
	}
}

func chainPublishDone(a, b func(PublishDone)) func(PublishDone) {
	switch {
	case a == nil:
		return b
	case b == nil:
		return a
	default:
		return func(info PublishDone) {
			safeCall(func() { a(info) })
			safeCall(func() { b(info) })
		}
	}
}

func chainHandlerError(a, b func(HandlerError)) func(HandlerError) {
	switch {
	case a == nil:
		return b
	case b == nil:
		return a
	default:
		return func(info HandlerError) {
			safeCall(func() { a(info) })
			safeCall(func() { b(info) })
		}
	}
}

func chainHandlerPanic(a, b func(HandlerPanic)) func(HandlerPanic) {
	switch {
	case a == nil:
		return b
	case b == nil:
		return a
	default:
		return func(info HandlerPanic) {
			safeCall(func() { a(info) })
			safeCall(func() { b(info) })
		}
	}
}

func chainDroppedEvent(a, b func(DroppedEvent)) func(DroppedEvent) {
	switch {
	case a == nil:
		return b
	case b == nil:
		return a
	default:
		return func(info DroppedEvent) {
			safeCall(func() { a(info) })
			safeCall(func() { b(info) })
		}
	}
}

func safeCall(fn func()) {
	defer func() {
		_ = recover()
	}()
	fn()
}
