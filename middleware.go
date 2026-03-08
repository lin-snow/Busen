package busen

import (
	"context"
	"fmt"
	"reflect"
)

// Dispatch carries untyped event metadata through middleware.
//
// Middleware is intentionally thin and local to in-process dispatch. It may
// inspect or transform the event metadata before the typed handler runs.
//
// Dispatch mutation rules are intentionally narrow:
//   - changes are visible to later middleware and the final handler
//   - changes do not rewrite hook payloads
//   - changes do not affect subscriber matching, publish-level hooks, or
//     async queue selection, all of which happen before middleware runs
type Dispatch struct {
	// EventType is the exact Go type being dispatched.
	EventType reflect.Type
	// Topic is the publish topic after publish options have been applied.
	Topic string
	// Key is the publish ordering key after publish options have been applied.
	Key string
	// Headers is a mutable copy of the publish headers for this handler call.
	Headers map[string]string
	// Meta is mutable structured metadata for this handler call.
	Meta map[string]string
	// Value is the event payload that will be passed to the typed handler.
	Value any
	// Async reports whether the target subscription is asynchronous.
	Async bool
}

// Next is the continuation function used by Middleware.
type Next func(context.Context, Dispatch) error

// Middleware wraps local handler dispatch in the same spirit as HTTP middleware.
type Middleware func(Next) Next

// Use registers global dispatch middleware.
//
// Middleware is applied to both sync and async handler execution. It does not
// replace hooks, and it does not manage bus lifecycle or routing.
func (b *Bus) Use(middlewares ...Middleware) error {
	if b == nil {
		return fmt.Errorf("%w: nil bus", ErrInvalidOption)
	}
	if b.gate.Closed() {
		return ErrClosed
	}
	if len(middlewares) == 0 {
		return nil
	}

	b.middlewareMu.Lock()
	defer b.middlewareMu.Unlock()

	combined := make([]Middleware, 0, len(b.middlewares)+len(middlewares))
	combined = append(combined, b.middlewares...)
	for _, middleware := range middlewares {
		if middleware == nil {
			return fmt.Errorf("%w: middleware is nil", ErrInvalidOption)
		}
		combined = append(combined, middleware)
	}
	b.middlewares = combined
	b.middleware = buildMiddlewareChain(combined)
	b.middlewareVersion.Add(1)
	return nil
}

func buildMiddlewareChain(middlewares []Middleware) func(Next) Next {
	if len(middlewares) == 0 {
		return nil
	}

	cached := append([]Middleware(nil), middlewares...)
	return func(next Next) Next {
		wrapped := next
		for i := len(cached) - 1; i >= 0; i-- {
			wrapped = cached[i](wrapped)
		}
		return wrapped
	}
}
