package busen

import (
	"context"
	"fmt"
	"sync/atomic"
	"testing"

	"github.com/lin-snow/Busen/internal/router"
)

// Benchmarks in this file focus on the main in-process hot paths:
// publish fan-out, topic routing, hooks, middleware, and async keyed delivery.

func BenchmarkPublishSync(b *testing.B) {
	for _, subs := range []int{1, 10, 100} {
		b.Run(fmt.Sprintf("subs_%d", subs), func(b *testing.B) {
			bus := New()
			for range subs {
				unsubscribe, err := Subscribe(bus, func(ctx context.Context, event Event[int]) error {
					return nil
				})
				if err != nil {
					b.Fatalf("Subscribe() error = %v", err)
				}
				defer unsubscribe()
			}

			ctx := context.Background()
			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				if err := Publish(ctx, bus, i); err != nil {
					b.Fatalf("Publish() error = %v", err)
				}
			}
		})
	}
}

func BenchmarkPublishAsyncSequential(b *testing.B) {
	bus := New()
	var processed atomic.Int64

	unsubscribe, err := Subscribe(bus, func(ctx context.Context, event Event[int]) error {
		processed.Add(1)
		return nil
	}, Async(), Sequential(), WithBuffer(4096))
	if err != nil {
		b.Fatalf("Subscribe() error = %v", err)
	}
	defer unsubscribe()

	ctx := context.Background()
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if err := Publish(ctx, bus, i); err != nil {
			b.Fatalf("Publish() error = %v", err)
		}
	}
	b.StopTimer()

	if err := bus.Close(context.Background()); err != nil {
		b.Fatalf("Close() error = %v", err)
	}
}

func BenchmarkPublishWithHooks(b *testing.B) {
	hooks := Hooks{
		OnPublishStart: func(PublishStart) {},
		OnPublishDone:  func(PublishDone) {},
	}

	for _, enabled := range []bool{false, true} {
		name := "disabled"
		opts := []Option(nil)
		if enabled {
			name = "enabled"
			opts = append(opts, WithHooks(hooks))
		}

		b.Run(name, func(b *testing.B) {
			bus := New(opts...)
			unsubscribe, err := Subscribe(bus, func(ctx context.Context, event Event[int]) error {
				return nil
			})
			if err != nil {
				b.Fatalf("Subscribe() error = %v", err)
			}
			defer unsubscribe()

			ctx := context.Background()
			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				if err := Publish(ctx, bus, i); err != nil {
					b.Fatalf("Publish() error = %v", err)
				}
			}
		})
	}
}

func BenchmarkPublishWithMiddleware(b *testing.B) {
	middleware := Middleware(func(next Next) Next {
		return func(ctx context.Context, dispatch Dispatch) error {
			return next(ctx, dispatch)
		}
	})

	for _, enabled := range []bool{false, true} {
		name := "disabled"
		bus := New()
		if enabled {
			name = "enabled"
			if err := bus.Use(middleware); err != nil {
				b.Fatalf("Use() error = %v", err)
			}
		}

		b.Run(name, func(b *testing.B) {
			unsubscribe, err := Subscribe(bus, func(ctx context.Context, event Event[int]) error {
				return nil
			})
			if err != nil {
				b.Fatalf("Subscribe() error = %v", err)
			}
			defer unsubscribe()

			ctx := context.Background()
			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				if err := Publish(ctx, bus, i); err != nil {
					b.Fatalf("Publish() error = %v", err)
				}
			}
		})
	}
}

func BenchmarkPublishWithMiddlewareAndHooks(b *testing.B) {
	bus := New(
		WithHooks(Hooks{
			OnPublishStart: func(PublishStart) {},
			OnPublishDone:  func(PublishDone) {},
		}),
		WithMiddleware(func(next Next) Next {
			return func(ctx context.Context, dispatch Dispatch) error {
				return next(ctx, dispatch)
			}
		}),
	)

	unsubscribe, err := Subscribe(bus, func(ctx context.Context, event Event[int]) error {
		return nil
	})
	if err != nil {
		b.Fatalf("Subscribe() error = %v", err)
	}
	defer unsubscribe()

	ctx := context.Background()
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if err := Publish(ctx, bus, i); err != nil {
			b.Fatalf("Publish() error = %v", err)
		}
	}
}

func BenchmarkPublishTopic(b *testing.B) {
	cases := []struct {
		name    string
		pattern string
		topic   string
	}{
		{name: "exact", pattern: "orders.created", topic: "orders.created"},
		{name: "wildcard", pattern: "orders.>", topic: "orders.eu.created"},
	}

	for _, tc := range cases {
		b.Run(tc.name, func(b *testing.B) {
			bus := New()
			unsubscribe, err := SubscribeTopic(bus, tc.pattern, func(ctx context.Context, event Event[int]) error {
				return nil
			})
			if err != nil {
				b.Fatalf("SubscribeTopic() error = %v", err)
			}
			defer unsubscribe()

			ctx := context.Background()
			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				if err := Publish(ctx, bus, i, WithTopic(tc.topic)); err != nil {
					b.Fatalf("Publish() error = %v", err)
				}
			}
		})
	}
}

func BenchmarkPublishAsyncKeyed(b *testing.B) {
	bus := New()
	var processed atomic.Int64

	unsubscribe, err := Subscribe(bus, func(ctx context.Context, event Event[int]) error {
		processed.Add(1)
		return nil
	}, Async(), WithParallelism(4), WithBuffer(4096))
	if err != nil {
		b.Fatalf("Subscribe() error = %v", err)
	}
	defer unsubscribe()

	ctx := context.Background()
	keys := []string{"alpha", "beta", "gamma", "delta"}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if err := Publish(ctx, bus, i, WithKey(keys[i%len(keys)])); err != nil {
			b.Fatalf("Publish() error = %v", err)
		}
	}
	b.StopTimer()

	if err := bus.Close(context.Background()); err != nil {
		b.Fatalf("Close() error = %v", err)
	}
}

func BenchmarkPublishAsyncKeyedTopic(b *testing.B) {
	bus := New()
	var processed atomic.Int64

	unsubscribe, err := SubscribeTopic(bus, "orders.>", func(ctx context.Context, event Event[int]) error {
		processed.Add(1)
		return nil
	}, Async(), WithParallelism(4), WithBuffer(4096))
	if err != nil {
		b.Fatalf("SubscribeTopic() error = %v", err)
	}
	defer unsubscribe()

	ctx := context.Background()
	keys := []string{"alpha", "beta", "gamma", "delta"}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if err := Publish(ctx, bus, i, WithTopic("orders.eu.created"), WithKey(keys[i%len(keys)])); err != nil {
			b.Fatalf("Publish() error = %v", err)
		}
	}
	b.StopTimer()

	if err := bus.Close(context.Background()); err != nil {
		b.Fatalf("Close() error = %v", err)
	}
}

func BenchmarkRouterMatch(b *testing.B) {
	cases := []struct {
		name    string
		pattern string
		topic   string
	}{
		{name: "exact", pattern: "orders.created", topic: "orders.created"},
		{name: "wildcard", pattern: "orders.>", topic: "orders.eu.created"},
	}

	for _, tc := range cases {
		b.Run(tc.name, func(b *testing.B) {
			matcher, err := router.Compile(tc.pattern)
			if err != nil {
				b.Fatalf("Compile() error = %v", err)
			}

			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				if !matcher.Match(tc.topic) {
					b.Fatal("matcher.Match() = false, want true")
				}
			}
		})
	}
}
