package busen

import (
	"context"
	"fmt"
	"reflect"
	"sync"
	"sync/atomic"

	intdispatch "github.com/lin-snow/Busen/internal/dispatch"
	"github.com/lin-snow/Busen/internal/router"
)

// Bus is a typed-first in-process event bus.
type Bus struct {
	cfg   config
	hooks Hooks
	gate  *intdispatch.Gate

	mu           sync.RWMutex
	subs         map[reflect.Type]map[uint64]*subscription
	middlewareMu sync.RWMutex
	middlewares  []Middleware

	nextID atomic.Uint64
}

type subscription struct {
	id        uint64
	eventType reflect.Type
	bus       *Bus

	matcher   router.Matcher
	predicate func(envelope) bool
	handler   func(context.Context, envelope) error
	hooks     Hooks

	async       bool
	parallelism int
	overflow    OverflowPolicy
	mailboxes   []chan workItem
	rr          atomic.Uint64

	queueMu  sync.Mutex
	gate     *intdispatch.Gate
	stopOnce sync.Once
	done     chan struct{}
}

type workItem struct {
	ctx context.Context
	env envelope
}

// New creates a new Bus.
func New(opts ...Option) *Bus {
	cfg := defaultConfig()
	for _, opt := range opts {
		if opt == nil {
			continue
		}
		if err := opt.apply(&cfg); err != nil {
			panic(err)
		}
	}

	return &Bus{
		cfg:         cfg,
		hooks:       cfg.hooks,
		gate:        intdispatch.NewGate(),
		subs:        make(map[reflect.Type]map[uint64]*subscription),
		middlewares: append([]Middleware(nil), cfg.middlewares...),
	}
}

// Close stops accepting new publishes and drains async subscribers.
func (b *Bus) Close(ctx context.Context) error {
	if ctx == nil {
		ctx = context.Background()
	}

	b.gate.Close()

	subs := b.allSubscriptions()
	for _, sub := range subs {
		sub.stopAccepting()
		sub.scheduleStop()
	}

	if err := b.gate.Wait(ctx); err != nil {
		return err
	}

	for _, sub := range subs {
		if err := sub.waitStopped(ctx); err != nil {
			return err
		}
	}

	return nil
}

func (b *Bus) allSubscriptions() []*subscription {
	b.mu.RLock()
	defer b.mu.RUnlock()

	out := make([]*subscription, 0, len(b.subs))
	for _, group := range b.subs {
		for _, sub := range group {
			out = append(out, sub)
		}
	}
	return out
}

func (b *Bus) addSubscription(eventType reflect.Type, sub *subscription) error {
	if b.gate.Closed() {
		return ErrClosed
	}

	b.mu.Lock()
	defer b.mu.Unlock()

	if b.gate.Closed() {
		return ErrClosed
	}

	group := b.subs[eventType]
	if group == nil {
		group = make(map[uint64]*subscription)
		b.subs[eventType] = group
	}
	group[sub.id] = sub
	return nil
}

func (b *Bus) removeSubscription(eventType reflect.Type, id uint64) {
	b.mu.Lock()
	defer b.mu.Unlock()

	group := b.subs[eventType]
	if group == nil {
		return
	}

	delete(group, id)
	if len(group) == 0 {
		delete(b.subs, eventType)
	}
}

func (b *Bus) snapshotSubscriptions(eventType reflect.Type) []*subscription {
	b.mu.RLock()
	defer b.mu.RUnlock()

	group := b.subs[eventType]
	if len(group) == 0 {
		return nil
	}

	out := make([]*subscription, 0, len(group))
	for _, sub := range group {
		out = append(out, sub)
	}
	return out
}

func newSubscription(
	bus *Bus,
	id uint64,
	eventType reflect.Type,
	matcher router.Matcher,
	predicate func(envelope) bool,
	handler func(context.Context, envelope) error,
	hooks Hooks,
	cfg subscribeConfig,
) *subscription {
	sub := &subscription{
		id:          id,
		eventType:   eventType,
		bus:         bus,
		matcher:     matcher,
		predicate:   predicate,
		handler:     handler,
		hooks:       hooks,
		async:       cfg.async,
		parallelism: cfg.parallelism,
		overflow:    cfg.overflow,
		gate:        intdispatch.NewGate(),
		done:        make(chan struct{}),
	}

	if sub.async {
		sub.mailboxes = make([]chan workItem, cfg.parallelism)
		for i := range sub.mailboxes {
			sub.mailboxes[i] = make(chan workItem, cfg.buffer)
		}
		sub.startWorkers()
	} else {
		close(sub.done)
	}

	return sub
}

func (s *subscription) matches(env envelope) bool {
	if s.matcher != nil && !s.matcher.Match(env.topic) {
		return false
	}
	if s.predicate != nil && !s.predicate(env) {
		return false
	}
	return true
}

func (s *subscription) deliver(ctx context.Context, env envelope) error {
	if !s.gate.Enter() {
		return nil
	}
	defer s.gate.Leave()

	if !s.async {
		return s.callHandler(ctx, env)
	}

	return s.enqueue(ctx, env)
}

func (s *subscription) enqueue(ctx context.Context, env envelope) error {
	item := workItem{ctx: ctx, env: env}
	mailbox := s.mailboxForKey(env.key)

	switch s.overflow {
	case OverflowBlock:
		select {
		case mailbox <- item:
			return nil
		case <-ctx.Done():
			return ctx.Err()
		}
	case OverflowFailFast:
		select {
		case mailbox <- item:
			return nil
		default:
			return ErrBufferFull
		}
	case OverflowDropNewest:
		select {
		case mailbox <- item:
			return nil
		default:
			s.onDropped(env, ErrDropped)
			return ErrDropped
		}
	case OverflowDropOldest:
		s.queueMu.Lock()
		defer s.queueMu.Unlock()

		select {
		case mailbox <- item:
			return nil
		default:
		}

		select {
		case dropped := <-mailbox:
			s.onDropped(dropped.env, ErrDropped)
		default:
			return ErrBufferFull
		}

		select {
		case mailbox <- item:
			return ErrDropped
		default:
			return ErrBufferFull
		}
	default:
		return fmt.Errorf("%w: unknown overflow policy", ErrInvalidOption)
	}
}

func (s *subscription) callHandler(ctx context.Context, env envelope) (err error) {
	dispatch := Dispatch{
		EventType: s.eventType,
		Topic:     env.topic,
		Key:       env.key,
		Headers:   cloneHeaders(env.headers),
		Value:     env.value,
		Async:     s.async,
	}

	defer func() {
		if recovered := recover(); recovered != nil {
			err = &HandlerPanicError{
				Value: recovered,
			}
			s.onPanic(envelope{
				topic:   dispatch.Topic,
				key:     dispatch.Key,
				value:   dispatch.Value,
				headers: dispatch.Headers,
			}, recovered)
		}
	}()

	err = s.invoke(ctx, dispatch)
	if err != nil {
		s.onError(envelope{
			topic:   dispatch.Topic,
			key:     dispatch.Key,
			value:   dispatch.Value,
			headers: dispatch.Headers,
		}, err)
	}
	return err
}

func (s *subscription) startWorkers() {
	var wg sync.WaitGroup
	for _, mailbox := range s.mailboxes {
		mailbox := mailbox
		wg.Add(1)
		go func() {
			defer wg.Done()
			for item := range mailbox {
				_ = s.callHandler(item.ctx, item.env)
			}
		}()
	}

	go func() {
		wg.Wait()
		close(s.done)
	}()
}

func (s *subscription) stopAccepting() {
	s.gate.Close()
}

func (s *subscription) scheduleStop() {
	if !s.async {
		return
	}

	s.stopOnce.Do(func() {
		go func() {
			_ = s.gate.Wait(context.Background())
			for _, mailbox := range s.mailboxes {
				close(mailbox)
			}
		}()
	})
}

func (s *subscription) waitStopped(ctx context.Context) error {
	if !s.async {
		return s.gate.Wait(ctx)
	}

	s.scheduleStop()

	select {
	case <-s.done:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (s *subscription) onError(env envelope, err error) {
	if s.hooks.OnHandlerError == nil {
		return
	}

	info := HandlerError{
		EventType: s.eventType,
		Topic:     env.topic,
		Key:       env.key,
		Async:     s.async,
		Err:       err,
	}
	safeCall(func() { s.hooks.OnHandlerError(info) })
}

func (s *subscription) onPanic(env envelope, value any) {
	if s.hooks.OnHandlerPanic == nil {
		return
	}

	info := HandlerPanic{
		EventType: s.eventType,
		Topic:     env.topic,
		Key:       env.key,
		Async:     s.async,
		Value:     value,
	}
	safeCall(func() { s.hooks.OnHandlerPanic(info) })
}

func (s *subscription) onDropped(env envelope, reason error) {
	if s.hooks.OnEventDropped == nil {
		return
	}

	info := DroppedEvent{
		EventType: s.eventType,
		Topic:     env.topic,
		Key:       env.key,
		Async:     true,
		Policy:    s.overflow,
		Reason:    reason,
	}
	safeCall(func() { s.hooks.OnEventDropped(info) })
}

func (s *subscription) mailboxForKey(key string) chan workItem {
	if len(s.mailboxes) == 1 {
		return s.mailboxes[0]
	}
	if key != "" {
		return s.mailboxes[int(hashString(key)%uint64(len(s.mailboxes)))]
	}
	next := s.rr.Add(1) - 1
	return s.mailboxes[int(next%uint64(len(s.mailboxes)))]
}

func (s *subscription) invoke(ctx context.Context, dispatch Dispatch) error {
	final := func(ctx context.Context, dispatch Dispatch) error {
		return s.handler(ctx, envelope{
			topic:   dispatch.Topic,
			key:     dispatch.Key,
			value:   dispatch.Value,
			headers: dispatch.Headers,
		})
	}

	chain := s.middlewareChain()
	if chain == nil {
		return final(ctx, dispatch)
	}
	return chain(final)(ctx, dispatch)
}

func (s *subscription) middlewareChain() func(Next) Next {
	bus := s.bus
	if bus == nil {
		return nil
	}

	bus.middlewareMu.RLock()
	defer bus.middlewareMu.RUnlock()

	if len(bus.middlewares) == 0 {
		return nil
	}

	middlewares := append([]Middleware(nil), bus.middlewares...)
	return func(next Next) Next {
		wrapped := next
		for i := len(middlewares) - 1; i >= 0; i-- {
			wrapped = middlewares[i](wrapped)
		}
		return wrapped
	}
}

type HandlerPanicError struct {
	Value any
}

func (e *HandlerPanicError) Error() string {
	return fmt.Sprintf("%s: %v", ErrHandlerPanic, e.Value)
}

func (e *HandlerPanicError) Unwrap() error {
	return ErrHandlerPanic
}

func hashString(value string) uint64 {
	const offset64 = 14695981039346656037
	const prime64 = 1099511628211

	hash := uint64(offset64)
	for i := 0; i < len(value); i++ {
		hash ^= uint64(value[i])
		hash *= prime64
	}
	return hash
}
