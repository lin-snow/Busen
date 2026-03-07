package busen

import (
	"context"
	"errors"
	"fmt"
	"reflect"
)

// Publish delivers a typed event to matching subscribers.
func Publish[T any](ctx context.Context, b *Bus, value T, opts ...PublishOption) error {
	if b == nil {
		return fmt.Errorf("%w: nil bus", ErrInvalidOption)
	}
	if ctx == nil {
		ctx = context.Background()
	}
	if !b.gate.Enter() {
		return ErrClosed
	}
	defer b.gate.Leave()

	cfg := publishConfig{}
	for _, opt := range opts {
		if opt == nil {
			continue
		}
		if err := opt.applyPublish(&cfg); err != nil {
			return err
		}
	}

	eventType := reflect.TypeFor[T]()
	if b.hooks.OnPublishStart != nil {
		info := PublishStart{
			EventType: eventType,
			Topic:     cfg.topic,
			Key:       cfg.key,
			Headers:   cloneHeaders(cfg.headers),
		}
		safeCall(func() { b.hooks.OnPublishStart(info) })
	}

	subs := b.snapshotSubscriptions(eventType)
	if len(subs) == 0 {
		if b.hooks.OnPublishDone != nil {
			info := PublishDone{
				EventType:          eventType,
				Topic:              cfg.topic,
				Key:                cfg.key,
				Headers:            cloneHeaders(cfg.headers),
				MatchedSubscribers: 0,
			}
			safeCall(func() { b.hooks.OnPublishDone(info) })
		}
		return nil
	}

	env := envelope{
		topic:   cfg.topic,
		key:     cfg.key,
		value:   value,
		headers: cloneHeaders(cfg.headers),
	}

	var errs []error
	matched := 0
	for _, sub := range subs {
		if !sub.matches(env) {
			continue
		}
		matched++
		if deliverErr := sub.deliver(ctx, env); deliverErr != nil {
			errs = append(errs, deliverErr)
		}
	}

	err := errors.Join(errs...)
	if b.hooks.OnPublishDone != nil {
		info := PublishDone{
			EventType:          eventType,
			Topic:              cfg.topic,
			Key:                cfg.key,
			Headers:            cloneHeaders(cfg.headers),
			MatchedSubscribers: matched,
			Err:                err,
		}
		safeCall(func() { b.hooks.OnPublishDone(info) })
	}

	return err
}
