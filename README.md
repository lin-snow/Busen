# Busen

Busen is a modern, typed-first EventBus for Go.

It is designed around plain Go values, `context.Context`, functional options, and explicit concurrency semantics. Type-based subscriptions are the primary API, while topics and wildcards are available as routing metadata when needed.

## Features

- Typed subscriptions with `Subscribe[T]` and `Publish[T]`
- Optional topic routing with `SubscribeTopic[T]`
- Predicate-based filtering with `SubscribeMatch[T]` and `WithFilter`
- Sync and async delivery modes
- Bounded async queues with explicit backpressure policies
- FIFO delivery for single-worker async subscribers
- Per-subscriber keyed ordering for async handlers
- Thin dispatch middleware for local composition
- Lightweight runtime hooks for publish, error, panic, and drop events
- Graceful shutdown with `Close(ctx)`

## Install

```bash
go get github.com/lin-snow/Busen
```

## Development

Common local commands are available through the `Makefile`:

```bash
make help
make fmt
make vet
make test
make test-race
make cover
make check
```

## Quick Start

```go
package main

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/lin-snow/Busen"
)

type UserCreated struct {
	ID    string
	Email string
}

func main() {
	bus := busen.New()

	unsubscribe, err := busen.Subscribe(bus, func(ctx context.Context, event busen.Event[UserCreated]) error {
		fmt.Printf("welcome %s\n", event.Value.Email)
		return nil
	})
	if err != nil {
		log.Fatal(err)
	}
	defer unsubscribe()

	err = busen.Publish(context.Background(), bus, UserCreated{
		ID:    "u_123",
		Email: "hello@example.com",
	})
	if err != nil {
		log.Fatal(err)
	}

	_ = bus.Close(context.Background())
}
```

## Topic Routing

Topics are dot-separated strings. Wildcards are intentionally small in scope:

- `*` matches exactly one segment
- `>` matches one or more remaining segments and must be the last segment

```go
sub, err := busen.SubscribeTopic(bus, "orders.>", func(ctx context.Context, event busen.Event[string]) error {
	fmt.Println(event.Topic, event.Value)
	return nil
})
if err != nil {
	log.Fatal(err)
}
defer sub()

_ = busen.Publish(context.Background(), bus, "created", busen.WithTopic("orders.eu.created"))
```

## Async Delivery

Async subscribers use bounded queues. Backpressure is explicit:

- `OverflowBlock`
- `OverflowFailFast`
- `OverflowDropNewest`
- `OverflowDropOldest`

```go
_, err = busen.Subscribe(bus, func(ctx context.Context, event busen.Event[UserCreated]) error {
	time.Sleep(50 * time.Millisecond)
	return nil
},
	busen.Async(),
	busen.Sequential(),
	busen.WithBuffer(128),
	busen.WithOverflow(busen.OverflowBlock),
)
```

If you also publish with `WithKey(...)`, async subscribers preserve ordering for
events that share the same non-empty key within that subscriber.

```go
_, err = busen.Subscribe(bus, func(ctx context.Context, event busen.Event[UserCreated]) error {
	return nil
}, busen.Async(), busen.WithParallelism(4), busen.WithBuffer(256))

_ = busen.Publish(context.Background(), bus, UserCreated{ID: "1"}, busen.WithKey("tenant-a"))
_ = busen.Publish(context.Background(), bus, UserCreated{ID: "2"}, busen.WithKey("tenant-a"))
```

Notes:

- key-based ordering only applies to async subscribers
- empty keys fall back to the regular non-keyed scheduling path
- ordering is per subscriber and per key, not global

## Middleware

Busen supports a thin middleware layer for local dispatch composition.

```go
err = bus.Use(func(next busen.Next) busen.Next {
	return func(ctx context.Context, dispatch busen.Dispatch) error {
		log.Printf("event=%v topic=%q", dispatch.EventType, dispatch.Topic)
		return next(ctx, dispatch)
	}
})
if err != nil {
	log.Fatal(err)
}
```

Middleware is intentionally limited:

- it wraps handler invocation only
- it does not replace hooks
- it does not manage retries, metrics, tracing, or distributed concerns
- it does not affect subscriber matching, topic routing, or async queue selection
- changes to `Dispatch` are visible to later middleware and the final handler
- hooks continue to report the original published metadata

If you prefer configuration at construction time, you can also use `WithMiddleware(...)`:

```go
bus := busen.New(
	busen.WithMiddleware(func(next busen.Next) busen.Next {
		return func(ctx context.Context, dispatch busen.Dispatch) error {
			return next(ctx, dispatch)
		}
	}),
)
```

## Hooks

Busen exposes a thin `Hooks` API for observing runtime events without turning the
core bus into a framework.

```go
bus := busen.New(
	busen.WithHooks(busen.Hooks{
		OnHandlerError: func(info busen.HandlerError) {
			log.Printf("async=%v topic=%q err=%v", info.Async, info.Topic, info.Err)
		},
		OnHandlerPanic: func(info busen.HandlerPanic) {
			log.Printf("panic in %v: %v", info.EventType, info.Value)
		},
		OnEventDropped: func(info busen.DroppedEvent) {
			log.Printf("dropped event for topic %q with policy %v", info.Topic, info.Policy)
		},
	}),
)
```

Hooks are notification points, not middleware:

- `OnPublishStart`
- `OnPublishDone`
- `OnHandlerError`
- `OnHandlerPanic`
- `OnEventDropped`

## Performance

Busen now includes repeatable benchmarks for the main hot paths:

- `Publish[T]` with 1 / 10 / 100 subscribers
- sync vs async sequential delivery
- async keyed delivery
- middleware enabled vs disabled
- middleware + hooks enabled
- async keyed delivery + topic routing
- hooks enabled vs disabled
- exact vs wildcard topic routing
- direct router matcher cost

Run them with:

```bash
go test ./... -run '^$' -bench . -benchmem
```

Current performance should be read as **in-process event bus** numbers, not
message broker throughput guarantees.

On a recent Apple Silicon machine, the current baseline is roughly:

- sync publish with 1 subscriber: about `130-145 ns/op`
- sync publish with 10 subscribers: about `500-530 ns/op`
- async sequential publish: about `250-265 ns/op`
- async keyed publish: about `300-305 ns/op`
- middleware-enabled publish: about `190-195 ns/op`
- middleware + hooks publish: about `235-240 ns/op`
- async keyed + topic publish: about `375-380 ns/op`
- exact topic publish: about `160 ns/op`
- wildcard topic publish: about `170 ns/op`

One data-backed optimization in this round was removing allocations from the
wildcard router match path. Direct wildcard matcher cost dropped to roughly
`7 ns/op` with `0 allocs/op`.

## Design Notes

- Type matching is exact. A subscription for one Go type does not receive another type automatically.
- Busen does not guarantee global ordering.
- FIFO is preserved only for a single async subscriber running with one worker.
- For async subscribers with `WithParallelism(n > 1)`, events with the same non-empty key preserve order within that subscriber.
- A middleware may rewrite dispatch metadata seen by later middleware and the handler, but routing, queue selection, and hooks still use the original published event.
- Sync handler errors are returned from `Publish`.
- Async handler errors and panics do not flow back to `Publish`; surface them via `Hooks`.
- Busen is meant for simple in-process application use. It is not a distributed event platform.
- Busen is an in-process event bus. Persistence, replay, and distributed transports are intentionally out of scope for `v1`.

## When To Use

Busen fits best when:

- you want typed in-process events inside a single Go application
- you need light topic routing and bounded async delivery
- you want a small library, not an event platform
- you prefer composing business policies outside the bus

Busen is not the right tool when:

- you need durability, replay, or cross-process delivery
- you need built-in tracing, metrics, retries, or rate limiting frameworks
- you need global ordering guarantees

## Project Docs

- Contributing guide: `CONTRIBUTING.md`
- Code of conduct: `CODE_OF_CONDUCT.md`
- Security policy: `SECURITY.md`
- Support guide: `SUPPORT.md`
- Governance: `GOVERNANCE.md`
- Release process: `RELEASING.md`
