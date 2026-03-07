package busen

import (
	"context"
	"fmt"
	"reflect"
	"sync"

	"github.com/lin-snow/Busen/internal/router"
)

// Subscribe registers a type-based subscription.
func Subscribe[T any](b *Bus, handler Handler[T], opts ...SubscribeOption) (func(), error) {
	return subscribeWithMatcher(b, nil, nil, handler, opts...)
}

// SubscribeTopic registers a type-based subscription constrained by a topic pattern.
func SubscribeTopic[T any](b *Bus, pattern string, handler Handler[T], opts ...SubscribeOption) (func(), error) {
	matcher, err := router.Compile(pattern)
	if err != nil {
		return nil, fmt.Errorf("%w: %s", ErrInvalidPattern, pattern)
	}

	return subscribeWithMatcher(b, matcher, nil, handler, opts...)
}

// SubscribeMatch registers a type-based subscription constrained by a predicate filter.
func SubscribeMatch[T any](b *Bus, match func(Event[T]) bool, handler Handler[T], opts ...SubscribeOption) (func(), error) {
	if match == nil {
		return nil, fmt.Errorf("%w: match predicate is nil", ErrInvalidOption)
	}

	predicate := func(env envelope) bool {
		return match(typedEvent[T](env))
	}

	return subscribeWithMatcher(b, nil, predicate, handler, opts...)
}

func subscribeWithMatcher[T any](
	b *Bus,
	matcher router.Matcher,
	basePredicate func(envelope) bool,
	handler Handler[T],
	opts ...SubscribeOption,
) (func(), error) {
	if b == nil {
		return nil, fmt.Errorf("%w: nil bus", ErrInvalidOption)
	}
	if handler == nil {
		return nil, ErrHandlerNil
	}

	cfg := defaultSubscribeConfig(b.cfg)
	for _, opt := range opts {
		if opt == nil {
			continue
		}
		if err := opt.applySubscribe(&cfg); err != nil {
			return nil, err
		}
	}

	if cfg.parallelism <= 0 {
		cfg.parallelism = 1
	}
	if cfg.buffer <= 0 {
		cfg.buffer = b.cfg.defaultBuffer
	}
	if !cfg.overflow.valid() {
		return nil, fmt.Errorf("%w: unknown overflow policy", ErrInvalidOption)
	}

	eventType := reflect.TypeFor[T]()
	predicate := basePredicate
	if cfg.filter != nil {
		if predicate == nil {
			predicate = cfg.filter
		} else {
			prev := predicate
			predicate = func(env envelope) bool {
				return prev(env) && cfg.filter(env)
			}
		}
	}

	runtimeHandler := func(ctx context.Context, env envelope) error {
		return handler(ctx, typedEvent[T](env))
	}

	id := b.nextID.Add(1)
	sub := newSubscription(b, id, eventType, matcher, predicate, runtimeHandler, b.hooks, cfg)
	if err := b.addSubscription(eventType, sub); err != nil {
		return nil, err
	}
	if sub.async {
		sub.startWorkers()
	}

	var once sync.Once
	return func() {
		once.Do(func() {
			sub.stopAccepting()
			b.removeSubscription(eventType, id)
			sub.scheduleStop()
		})
	}, nil
}
