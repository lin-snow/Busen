package busen

import (
	"context"
	"fmt"
)

// OverflowPolicy controls what happens when an async subscriber queue is full.
type OverflowPolicy int

const (
	// OverflowBlock blocks the publisher until queue space is available.
	OverflowBlock OverflowPolicy = iota
	// OverflowFailFast returns an error instead of waiting for queue space.
	OverflowFailFast
	// OverflowDropNewest drops the incoming event when the queue is full.
	OverflowDropNewest
	// OverflowDropOldest evicts the oldest queued event to admit the new event.
	OverflowDropOldest
)

// Handler handles a typed event.
type Handler[T any] func(ctx context.Context, event Event[T]) error

type config struct {
	defaultBuffer   int
	defaultOverflow OverflowPolicy
	hooks           Hooks
	middlewares     []Middleware
	metadataBuilder MetadataBuilder
}

type subscribeConfig struct {
	async       bool
	buffer      int
	parallelism int
	overflow    OverflowPolicy
	filter      func(envelope) bool
}

type publishConfig struct {
	topic   string
	key     string
	headers map[string]string
	meta    map[string]string
}

// Option configures a Bus.
//
// Callers typically obtain Option values from helpers such as
// [WithDefaultBuffer], [WithDefaultOverflow], [WithHooks], and
// [WithMiddleware] rather than implementing this interface directly.
type Option interface {
	apply(*config) error
}

// PublishOption configures a publish call.
//
// Callers typically obtain PublishOption values from helpers such as
// [WithTopic], [WithKey], and [WithHeaders] rather than implementing this
// interface directly.
type PublishOption interface {
	applyPublish(*publishConfig) error
}

// SubscribeOption configures a subscription.
//
// Callers typically obtain SubscribeOption values from helpers such as
// [Async], [Sequential], [WithParallelism], [WithBuffer], [WithOverflow], and
// [WithFilter] rather than implementing this interface directly.
type SubscribeOption interface {
	applySubscribe(*subscribeConfig) error
}

type optionFunc func(*config) error

func (f optionFunc) apply(cfg *config) error {
	return f(cfg)
}

type publishOptionFunc func(*publishConfig) error

func (f publishOptionFunc) applyPublish(cfg *publishConfig) error {
	return f(cfg)
}

type subscribeOptionFunc func(*subscribeConfig) error

func (f subscribeOptionFunc) applySubscribe(cfg *subscribeConfig) error {
	return f(cfg)
}

// WithDefaultBuffer sets the default queue size for async subscribers.
func WithDefaultBuffer(size int) Option {
	return optionFunc(func(cfg *config) error {
		if size <= 0 {
			return fmt.Errorf("%w: default buffer must be > 0", ErrInvalidOption)
		}
		cfg.defaultBuffer = size
		return nil
	})
}

// WithDefaultOverflow sets the default overflow policy for async subscribers.
func WithDefaultOverflow(policy OverflowPolicy) Option {
	return optionFunc(func(cfg *config) error {
		if !policy.valid() {
			return fmt.Errorf("%w: unknown overflow policy", ErrInvalidOption)
		}
		cfg.defaultOverflow = policy
		return nil
	})
}

// WithHooks registers runtime hooks for publish and handler lifecycle events.
func WithHooks(hooks Hooks) Option {
	return optionFunc(func(cfg *config) error {
		mergeHooks(&cfg.hooks, hooks)
		return nil
	})
}

// WithMiddleware registers global dispatch middleware at bus construction time.
func WithMiddleware(middlewares ...Middleware) Option {
	return optionFunc(func(cfg *config) error {
		for _, middleware := range middlewares {
			if middleware == nil {
				return fmt.Errorf("%w: middleware is nil", ErrInvalidOption)
			}
			cfg.middlewares = append(cfg.middlewares, middleware)
		}
		return nil
	})
}

// WithMetadataBuilder registers a global metadata builder for publish envelopes.
func WithMetadataBuilder(builder MetadataBuilder) Option {
	return optionFunc(func(cfg *config) error {
		if builder == nil {
			return fmt.Errorf("%w: metadata builder is nil", ErrInvalidOption)
		}
		cfg.metadataBuilder = builder
		return nil
	})
}

// WithTopic sets the routing topic for a published event.
func WithTopic(topic string) PublishOption {
	return publishOptionFunc(func(cfg *publishConfig) error {
		cfg.topic = topic
		return nil
	})
}

// WithKey sets an optional ordering key for a published event.
//
// In async mode, subscribers with multiple workers preserve ordering for events
// that share the same non-empty key within that subscriber. Empty keys fall back
// to the regular non-keyed path.
func WithKey(key string) PublishOption {
	return publishOptionFunc(func(cfg *publishConfig) error {
		cfg.key = key
		return nil
	})
}

// WithHeaders sets headers for a published event.
func WithHeaders(headers map[string]string) PublishOption {
	return publishOptionFunc(func(cfg *publishConfig) error {
		cfg.headers = cloneHeaders(headers)
		return nil
	})
}

// WithMetadata sets structured envelope metadata for a published event.
func WithMetadata(meta map[string]string) PublishOption {
	return publishOptionFunc(func(cfg *publishConfig) error {
		cfg.meta = cloneHeaders(meta)
		return nil
	})
}

// Async delivers events through a bounded queue and worker goroutines.
func Async() SubscribeOption {
	return subscribeOptionFunc(func(cfg *subscribeConfig) error {
		cfg.async = true
		return nil
	})
}

// Sequential is shorthand for single-worker async FIFO delivery.
//
// It enables async delivery and forces the subscriber to run with one worker.
func Sequential() SubscribeOption {
	return subscribeOptionFunc(func(cfg *subscribeConfig) error {
		cfg.async = true
		cfg.parallelism = 1
		return nil
	})
}

// WithParallelism sets the number of workers for an async subscriber.
func WithParallelism(n int) SubscribeOption {
	return subscribeOptionFunc(func(cfg *subscribeConfig) error {
		if n <= 0 {
			return fmt.Errorf("%w: parallelism must be > 0", ErrInvalidOption)
		}
		cfg.async = true
		cfg.parallelism = n
		return nil
	})
}

// WithBuffer sets the queue size for an async subscriber.
func WithBuffer(size int) SubscribeOption {
	return subscribeOptionFunc(func(cfg *subscribeConfig) error {
		if size <= 0 {
			return fmt.Errorf("%w: buffer must be > 0", ErrInvalidOption)
		}
		cfg.async = true
		cfg.buffer = size
		return nil
	})
}

// WithOverflow sets the queue overflow policy for an async subscriber.
func WithOverflow(policy OverflowPolicy) SubscribeOption {
	return subscribeOptionFunc(func(cfg *subscribeConfig) error {
		if !policy.valid() {
			return fmt.Errorf("%w: unknown overflow policy", ErrInvalidOption)
		}
		cfg.async = true
		cfg.overflow = policy
		return nil
	})
}

// WithFilter applies a predicate filter before the handler runs.
func WithFilter[T any](fn func(Event[T]) bool) SubscribeOption {
	return subscribeOptionFunc(func(cfg *subscribeConfig) error {
		if fn == nil {
			return fmt.Errorf("%w: filter is nil", ErrInvalidOption)
		}

		next := func(env envelope) bool {
			return fn(typedEvent[T](env))
		}

		if cfg.filter == nil {
			cfg.filter = next
			return nil
		}

		prev := cfg.filter
		cfg.filter = func(env envelope) bool {
			return prev(env) && next(env)
		}
		return nil
	})
}

func defaultConfig() config {
	return config{
		defaultBuffer:   64,
		defaultOverflow: OverflowBlock,
	}
}

func defaultSubscribeConfig(cfg config) subscribeConfig {
	return subscribeConfig{
		buffer:      cfg.defaultBuffer,
		parallelism: 1,
		overflow:    cfg.defaultOverflow,
	}
}

func (p OverflowPolicy) valid() bool {
	return p >= OverflowBlock && p <= OverflowDropOldest
}
