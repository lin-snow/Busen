package busen

import (
	"reflect"
)

// Hooks observes publish and handler lifecycle events.
//
// Hooks are intentionally thin. They are not a full middleware pipeline and do
// not change delivery semantics. They exist to observe important runtime events
// such as async failures, panics, and dropped events.
type Hooks struct {
	// OnPublishStart runs before matching subscribers are evaluated.
	OnPublishStart func(PublishStart)
	// OnPublishDone runs after all matching deliveries have been attempted.
	OnPublishDone func(PublishDone)
	// OnHandlerError runs when a handler returns a non-nil error.
	OnHandlerError func(HandlerError)
	// OnHandlerPanic runs when a handler panic is recovered.
	OnHandlerPanic func(HandlerPanic)
	// OnEventDropped runs when async backpressure drops an event.
	OnEventDropped func(DroppedEvent)
	// OnHookPanic runs when another hook panics and the panic is recovered.
	OnHookPanic func(HookPanic)
}

// PublishStart describes the beginning of a publish operation.
type PublishStart struct {
	// EventType is the exact Go type being published.
	EventType reflect.Type
	// Topic is the publish topic after options have been applied.
	Topic string
	// Key is the publish ordering key after options have been applied.
	Key string
	// Headers is a copy of the publish headers.
	Headers map[string]string
}

// PublishDone describes the end of a publish operation.
type PublishDone struct {
	// EventType is the exact Go type that was published.
	EventType reflect.Type
	// Topic is the publish topic after options have been applied.
	Topic string
	// Key is the publish ordering key after options have been applied.
	Key string
	// Headers is a copy of the publish headers.
	Headers map[string]string
	// MatchedSubscribers is the number of subscriptions whose routing constraints
	// matched the published event.
	MatchedSubscribers int
	// DeliveredSubscribers is the number of subscriptions that accepted the event
	// for handler execution or async enqueue after lifecycle checks.
	DeliveredSubscribers int
	// Err joins delivery errors returned during publish, if any.
	Err error
}

// HandlerError describes a handler error.
type HandlerError struct {
	// EventType is the exact Go type handled by the subscriber.
	EventType reflect.Type
	// Topic is the event topic seen by the handler.
	Topic string
	// Key is the event ordering key seen by the handler.
	Key string
	// Async reports whether the handler ran in async mode.
	Async bool
	// Err is the error returned by the handler.
	Err error
}

// HandlerPanic describes a recovered handler panic.
type HandlerPanic struct {
	// EventType is the exact Go type handled by the subscriber.
	EventType reflect.Type
	// Topic is the event topic seen by the handler.
	Topic string
	// Key is the event ordering key seen by the handler.
	Key string
	// Async reports whether the handler ran in async mode.
	Async bool
	// Value is the recovered panic value.
	Value any
}

// DroppedEvent describes a dropped event caused by backpressure.
type DroppedEvent struct {
	// EventType is the exact Go type that could not be queued.
	EventType reflect.Type
	// Topic is the event topic that was being delivered.
	Topic string
	// Key is the event ordering key that was being delivered.
	Key string
	// Async is always true for dropped events.
	Async bool
	// Policy is the overflow policy that decided the drop behavior.
	Policy OverflowPolicy
	// Reason reports why the event was dropped.
	Reason error
}

// HookPanic describes a recovered panic raised by another hook callback.
type HookPanic struct {
	// Hook is the callback name that panicked, such as "OnPublishDone".
	Hook string
	// Value is the recovered panic value.
	Value any
}

func mergeHooks(dst *Hooks, src Hooks) {
	if dst == nil {
		return
	}

	dst.OnHookPanic = chainHookPanic(dst.OnHookPanic, src.OnHookPanic)
	dst.OnPublishStart = chainPublishStart(dst, dst.OnPublishStart, src.OnPublishStart)
	dst.OnPublishDone = chainPublishDone(dst, dst.OnPublishDone, src.OnPublishDone)
	dst.OnHandlerError = chainHandlerError(dst, dst.OnHandlerError, src.OnHandlerError)
	dst.OnHandlerPanic = chainHandlerPanic(dst, dst.OnHandlerPanic, src.OnHandlerPanic)
	dst.OnEventDropped = chainDroppedEvent(dst, dst.OnEventDropped, src.OnEventDropped)
}

func chainPublishStart(dst *Hooks, a, b func(PublishStart)) func(PublishStart) {
	switch {
	case a == nil:
		return b
	case b == nil:
		return a
	default:
		return func(info PublishStart) {
			safeCall("OnPublishStart", hookPanicReporter(dst), func() { a(info) })
			safeCall("OnPublishStart", hookPanicReporter(dst), func() { b(info) })
		}
	}
}

func chainPublishDone(dst *Hooks, a, b func(PublishDone)) func(PublishDone) {
	switch {
	case a == nil:
		return b
	case b == nil:
		return a
	default:
		return func(info PublishDone) {
			safeCall("OnPublishDone", hookPanicReporter(dst), func() { a(info) })
			safeCall("OnPublishDone", hookPanicReporter(dst), func() { b(info) })
		}
	}
}

func chainHandlerError(dst *Hooks, a, b func(HandlerError)) func(HandlerError) {
	switch {
	case a == nil:
		return b
	case b == nil:
		return a
	default:
		return func(info HandlerError) {
			safeCall("OnHandlerError", hookPanicReporter(dst), func() { a(info) })
			safeCall("OnHandlerError", hookPanicReporter(dst), func() { b(info) })
		}
	}
}

func chainHandlerPanic(dst *Hooks, a, b func(HandlerPanic)) func(HandlerPanic) {
	switch {
	case a == nil:
		return b
	case b == nil:
		return a
	default:
		return func(info HandlerPanic) {
			safeCall("OnHandlerPanic", hookPanicReporter(dst), func() { a(info) })
			safeCall("OnHandlerPanic", hookPanicReporter(dst), func() { b(info) })
		}
	}
}

func chainDroppedEvent(dst *Hooks, a, b func(DroppedEvent)) func(DroppedEvent) {
	switch {
	case a == nil:
		return b
	case b == nil:
		return a
	default:
		return func(info DroppedEvent) {
			safeCall("OnEventDropped", hookPanicReporter(dst), func() { a(info) })
			safeCall("OnEventDropped", hookPanicReporter(dst), func() { b(info) })
		}
	}
}

func chainHookPanic(a, b func(HookPanic)) func(HookPanic) {
	switch {
	case a == nil:
		return b
	case b == nil:
		return a
	default:
		return func(info HookPanic) {
			safeCall("OnHookPanic", nil, func() { a(info) })
			safeCall("OnHookPanic", nil, func() { b(info) })
		}
	}
}

func hookPanicReporter(hooks *Hooks) func(HookPanic) {
	if hooks == nil {
		return nil
	}

	return func(info HookPanic) {
		if hooks.OnHookPanic == nil {
			return
		}
		safeCall("OnHookPanic", nil, func() { hooks.OnHookPanic(info) })
	}
}

func safeCall(name string, report func(HookPanic), fn func()) {
	defer func() {
		if recovered := recover(); recovered != nil && report != nil {
			report(HookPanic{
				Hook:  name,
				Value: recovered,
			})
		}
	}()
	fn()
}
