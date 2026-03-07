package busen

import (
	"context"
	"errors"
	"reflect"
	"runtime"
	"slices"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestPublishSubscribeTyped(t *testing.T) {
	bus := New()

	got := make(chan int, 1)
	unsubscribe, err := Subscribe(bus, func(_ context.Context, event Event[int]) error {
		got <- event.Value
		return nil
	})
	if err != nil {
		t.Fatalf("Subscribe() error = %v", err)
	}
	defer unsubscribe()

	if err := Publish(context.Background(), bus, 42); err != nil {
		t.Fatalf("Publish() error = %v", err)
	}

	select {
	case value := <-got:
		if value != 42 {
			t.Fatalf("got %d, want 42", value)
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for event")
	}
}

func TestTypeIsolation(t *testing.T) {
	bus := New()

	var ints atomic.Int64
	var strings atomic.Int64

	unsubInt, err := Subscribe(bus, func(_ context.Context, _ Event[int]) error {
		ints.Add(1)
		return nil
	})
	if err != nil {
		t.Fatalf("Subscribe[int]() error = %v", err)
	}
	defer unsubInt()

	unsubString, err := Subscribe(bus, func(_ context.Context, _ Event[string]) error {
		strings.Add(1)
		return nil
	})
	if err != nil {
		t.Fatalf("Subscribe[string]() error = %v", err)
	}
	defer unsubString()

	if err := Publish(context.Background(), bus, 7); err != nil {
		t.Fatalf("Publish() error = %v", err)
	}

	if ints.Load() != 1 {
		t.Fatalf("int handler called %d times, want 1", ints.Load())
	}
	if strings.Load() != 0 {
		t.Fatalf("string handler called %d times, want 0", strings.Load())
	}
}

func TestSubscribeMatch(t *testing.T) {
	bus := New()

	var got []int
	var mu sync.Mutex

	unsubscribe, err := SubscribeMatch(bus, func(event Event[int]) bool {
		return event.Value%2 == 0
	}, func(_ context.Context, event Event[int]) error {
		mu.Lock()
		defer mu.Unlock()
		got = append(got, event.Value)
		return nil
	})
	if err != nil {
		t.Fatalf("SubscribeMatch() error = %v", err)
	}
	defer unsubscribe()

	for _, value := range []int{1, 2, 3, 4} {
		if err := Publish(context.Background(), bus, value); err != nil {
			t.Fatalf("Publish(%d) error = %v", value, err)
		}
	}

	if !reflect.DeepEqual(got, []int{2, 4}) {
		t.Fatalf("got %v, want [2 4]", got)
	}
}

func TestMiddlewareWrapsHandler(t *testing.T) {
	bus := New()
	var calls []string

	err := bus.Use(func(next Next) Next {
		return func(ctx context.Context, dispatch Dispatch) error {
			calls = append(calls, "before")
			err := next(ctx, dispatch)
			calls = append(calls, "after")
			return err
		}
	})
	if err != nil {
		t.Fatalf("Use() error = %v", err)
	}

	unsubscribe, err := Subscribe(bus, func(_ context.Context, event Event[int]) error {
		calls = append(calls, "handler")
		return nil
	})
	if err != nil {
		t.Fatalf("Subscribe() error = %v", err)
	}
	defer unsubscribe()

	if err := Publish(context.Background(), bus, 1); err != nil {
		t.Fatalf("Publish() error = %v", err)
	}

	if !reflect.DeepEqual(calls, []string{"before", "handler", "after"}) {
		t.Fatalf("calls = %v, want [before handler after]", calls)
	}
}

func TestMiddlewareCanTransformDispatch(t *testing.T) {
	bus := New()

	err := bus.Use(func(next Next) Next {
		return func(ctx context.Context, dispatch Dispatch) error {
			dispatch.Topic = "renamed.topic"
			if dispatch.Headers == nil {
				dispatch.Headers = map[string]string{}
			}
			dispatch.Headers["from-middleware"] = "yes"
			return next(ctx, dispatch)
		}
	})
	if err != nil {
		t.Fatalf("Use() error = %v", err)
	}

	got := make(chan Event[string], 1)
	unsubscribe, err := SubscribeTopic(bus, "source.topic", func(_ context.Context, event Event[string]) error {
		got <- event
		return nil
	})
	if err != nil {
		t.Fatalf("SubscribeTopic() error = %v", err)
	}
	defer unsubscribe()

	if err := Publish(context.Background(), bus, "hello", WithTopic("source.topic")); err != nil {
		t.Fatalf("Publish() error = %v", err)
	}

	select {
	case event := <-got:
		if event.Topic != "renamed.topic" {
			t.Fatalf("event.Topic = %q, want renamed.topic", event.Topic)
		}
		if event.Headers["from-middleware"] != "yes" {
			t.Fatalf("event.Headers = %v, want middleware header", event.Headers)
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for middleware-transformed event")
	}
}

func TestWithMiddlewareRegistersGlobalMiddleware(t *testing.T) {
	var calls []string
	bus := New(WithMiddleware(func(next Next) Next {
		return func(ctx context.Context, dispatch Dispatch) error {
			calls = append(calls, "middleware")
			return next(ctx, dispatch)
		}
	}))

	unsubscribe, err := Subscribe(bus, func(_ context.Context, _ Event[int]) error {
		calls = append(calls, "handler")
		return nil
	})
	if err != nil {
		t.Fatalf("Subscribe() error = %v", err)
	}
	defer unsubscribe()

	if err := Publish(context.Background(), bus, 1); err != nil {
		t.Fatalf("Publish() error = %v", err)
	}

	if !reflect.DeepEqual(calls, []string{"middleware", "handler"}) {
		t.Fatalf("calls = %v, want [middleware handler]", calls)
	}
}

func TestUseAfterSubscribeAppliesToExistingSubscriptions(t *testing.T) {
	bus := New()

	var calls []string
	unsubscribe, err := Subscribe(bus, func(_ context.Context, _ Event[int]) error {
		calls = append(calls, "handler")
		return nil
	})
	if err != nil {
		t.Fatalf("Subscribe() error = %v", err)
	}
	defer unsubscribe()

	if err := bus.Use(func(next Next) Next {
		return func(ctx context.Context, dispatch Dispatch) error {
			calls = append(calls, "middleware")
			return next(ctx, dispatch)
		}
	}); err != nil {
		t.Fatalf("Use() error = %v", err)
	}

	if err := Publish(context.Background(), bus, 1); err != nil {
		t.Fatalf("Publish() error = %v", err)
	}

	if !reflect.DeepEqual(calls, []string{"middleware", "handler"}) {
		t.Fatalf("calls = %v, want [middleware handler]", calls)
	}
}

func TestMiddlewareDoesNotAffectPublishHooksOrRouting(t *testing.T) {
	startCh := make(chan PublishStart, 1)
	doneCh := make(chan PublishDone, 1)
	got := make(chan Event[string], 1)

	bus := New(
		WithHooks(Hooks{
			OnPublishStart: func(info PublishStart) { startCh <- info },
			OnPublishDone:  func(info PublishDone) { doneCh <- info },
		}),
		WithMiddleware(func(next Next) Next {
			return func(ctx context.Context, dispatch Dispatch) error {
				dispatch.Topic = "changed.topic"
				dispatch.Key = "changed-key"
				return next(ctx, dispatch)
			}
		}),
	)

	unsubscribe, err := SubscribeTopic(bus, "original.topic", func(_ context.Context, event Event[string]) error {
		got <- event
		return nil
	})
	if err != nil {
		t.Fatalf("SubscribeTopic() error = %v", err)
	}
	defer unsubscribe()

	if err := Publish(context.Background(), bus, "hello", WithTopic("original.topic"), WithKey("original-key")); err != nil {
		t.Fatalf("Publish() error = %v", err)
	}

	select {
	case info := <-startCh:
		if info.Topic != "original.topic" || info.Key != "original-key" {
			t.Fatalf("publish start info = %+v, want original metadata", info)
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for publish start hook")
	}

	select {
	case info := <-doneCh:
		if info.Topic != "original.topic" || info.Key != "original-key" {
			t.Fatalf("publish done info = %+v, want original metadata", info)
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for publish done hook")
	}

	select {
	case event := <-got:
		if event.Topic != "changed.topic" || event.Key != "changed-key" {
			t.Fatalf("event = %+v, want changed metadata", event)
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for transformed event")
	}
}

func TestMiddlewareDoesNotRewriteHandlerErrorHooks(t *testing.T) {
	errCh := make(chan HandlerError, 1)

	bus := New(
		WithHooks(Hooks{
			OnHandlerError: func(info HandlerError) { errCh <- info },
		}),
		WithMiddleware(func(next Next) Next {
			return func(ctx context.Context, dispatch Dispatch) error {
				dispatch.Topic = "error.topic"
				dispatch.Key = "error-key"
				return next(ctx, dispatch)
			}
		}),
	)

	unsubscribe, err := Subscribe(bus, func(_ context.Context, _ Event[int]) error {
		return errors.New("boom")
	})
	if err != nil {
		t.Fatalf("Subscribe() error = %v", err)
	}
	defer unsubscribe()

	if err := Publish(context.Background(), bus, 1, WithTopic("source.topic"), WithKey("source-key")); err == nil {
		t.Fatal("Publish() error = nil, want non-nil")
	}

	select {
	case info := <-errCh:
		if info.Topic != "source.topic" || info.Key != "source-key" {
			t.Fatalf("handler error info = %+v, want original metadata", info)
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for handler error hook")
	}
}

func TestAsyncSequentialPreservesOrder(t *testing.T) {
	bus := New()

	var (
		mu     sync.Mutex
		values []int
		wg     sync.WaitGroup
	)

	wg.Add(5)
	unsubscribe, err := Subscribe(bus, func(ctx context.Context, event Event[int]) error {
		defer wg.Done()
		mu.Lock()
		values = append(values, event.Value)
		mu.Unlock()
		return nil
	}, Async(), Sequential(), WithBuffer(8))
	if err != nil {
		t.Fatalf("Subscribe() error = %v", err)
	}
	defer unsubscribe()

	for i := range 5 {
		if err := Publish(context.Background(), bus, i); err != nil {
			t.Fatalf("Publish(%d) error = %v", i, err)
		}
	}

	done := make(chan struct{})
	go func() {
		wg.Wait()
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for sequential handler")
	}

	if !slices.Equal(values, []int{0, 1, 2, 3, 4}) {
		t.Fatalf("got %v, want [0 1 2 3 4]", values)
	}
}

func TestAsyncParallelPreservesOrderPerKey(t *testing.T) {
	bus := New()

	var (
		mu    sync.Mutex
		alpha []int
		beta  []int
		wg    sync.WaitGroup
	)

	wg.Add(6)
	unsubscribe, err := Subscribe(bus, func(ctx context.Context, event Event[int]) error {
		defer wg.Done()
		mu.Lock()
		defer mu.Unlock()
		switch event.Key {
		case "alpha":
			alpha = append(alpha, event.Value)
		case "beta":
			beta = append(beta, event.Value)
		}
		return nil
	}, Async(), WithParallelism(2), WithBuffer(8))
	if err != nil {
		t.Fatalf("Subscribe() error = %v", err)
	}
	defer unsubscribe()

	publishes := []struct {
		key   string
		value int
	}{
		{"alpha", 1},
		{"beta", 10},
		{"alpha", 2},
		{"beta", 11},
		{"alpha", 3},
		{"beta", 12},
	}
	for _, publish := range publishes {
		if err := Publish(context.Background(), bus, publish.value, WithKey(publish.key)); err != nil {
			t.Fatalf("Publish(%s, %d) error = %v", publish.key, publish.value, err)
		}
	}

	done := make(chan struct{})
	go func() {
		wg.Wait()
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for keyed async handler")
	}

	if !slices.Equal(alpha, []int{1, 2, 3}) {
		t.Fatalf("alpha = %v, want [1 2 3]", alpha)
	}
	if !slices.Equal(beta, []int{10, 11, 12}) {
		t.Fatalf("beta = %v, want [10 11 12]", beta)
	}
}

func TestAsyncParallelEmptyKeyDeliversAll(t *testing.T) {
	bus := New()

	var calls atomic.Int64
	var wg sync.WaitGroup
	wg.Add(6)

	unsubscribe, err := Subscribe(bus, func(ctx context.Context, event Event[int]) error {
		calls.Add(1)
		wg.Done()
		return nil
	}, Async(), WithParallelism(3), WithBuffer(8))
	if err != nil {
		t.Fatalf("Subscribe() error = %v", err)
	}
	defer unsubscribe()

	for i := range 6 {
		if err := Publish(context.Background(), bus, i); err != nil {
			t.Fatalf("Publish(%d) error = %v", i, err)
		}
	}

	done := make(chan struct{})
	go func() {
		wg.Wait()
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for async empty-key events")
	}

	if calls.Load() != 6 {
		t.Fatalf("calls = %d, want 6", calls.Load())
	}
}

func TestAsyncParallelPreservesOrderPerKeyWithMiddlewareAndHooks(t *testing.T) {
	doneCh := make(chan PublishDone, 4)
	bus := New(
		WithHooks(Hooks{
			OnPublishDone: func(info PublishDone) {
				doneCh <- info
			},
		}),
		WithMiddleware(func(next Next) Next {
			return func(ctx context.Context, dispatch Dispatch) error {
				if dispatch.Headers == nil {
					dispatch.Headers = map[string]string{}
				}
				dispatch.Headers["mw"] = "yes"
				return next(ctx, dispatch)
			}
		}),
	)

	var (
		mu    sync.Mutex
		alpha []int
		beta  []int
		wg    sync.WaitGroup
	)
	wg.Add(4)

	unsubscribe, err := Subscribe(bus, func(ctx context.Context, event Event[int]) error {
		defer wg.Done()
		if event.Headers["mw"] != "yes" {
			t.Fatalf("event.Headers = %v, want middleware header", event.Headers)
		}
		mu.Lock()
		defer mu.Unlock()
		switch event.Key {
		case "alpha":
			alpha = append(alpha, event.Value)
		case "beta":
			beta = append(beta, event.Value)
		}
		return nil
	}, Async(), WithParallelism(2), WithBuffer(8))
	if err != nil {
		t.Fatalf("Subscribe() error = %v", err)
	}
	defer unsubscribe()

	publishes := []struct {
		key   string
		value int
	}{
		{"alpha", 1},
		{"beta", 10},
		{"alpha", 2},
		{"beta", 11},
	}
	for _, publish := range publishes {
		if err := Publish(context.Background(), bus, publish.value, WithKey(publish.key)); err != nil {
			t.Fatalf("Publish(%s, %d) error = %v", publish.key, publish.value, err)
		}
	}

	waitDone := make(chan struct{})
	go func() {
		wg.Wait()
		close(waitDone)
	}()

	select {
	case <-waitDone:
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for combined keyed ordering test")
	}

	if !slices.Equal(alpha, []int{1, 2}) {
		t.Fatalf("alpha = %v, want [1 2]", alpha)
	}
	if !slices.Equal(beta, []int{10, 11}) {
		t.Fatalf("beta = %v, want [10 11]", beta)
	}

	for range 4 {
		select {
		case info := <-doneCh:
			if info.Err != nil {
				t.Fatalf("publish done err = %v, want nil", info.Err)
			}
		case <-time.After(time.Second):
			t.Fatal("timed out waiting for publish done hook")
		}
	}
}

func TestAsyncOverflowFailFast(t *testing.T) {
	bus := New()

	started := make(chan struct{}, 1)
	release := make(chan struct{})

	unsubscribe, err := Subscribe(bus, func(ctx context.Context, event Event[int]) error {
		select {
		case started <- struct{}{}:
		default:
		}
		<-release
		return nil
	}, Async(), WithBuffer(1), WithOverflow(OverflowFailFast))
	if err != nil {
		t.Fatalf("Subscribe() error = %v", err)
	}
	defer unsubscribe()

	if err := Publish(context.Background(), bus, 1); err != nil {
		t.Fatalf("first publish error = %v", err)
	}

	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for handler start")
	}

	if err := Publish(context.Background(), bus, 2); err != nil {
		t.Fatalf("second publish error = %v", err)
	}

	err = Publish(context.Background(), bus, 3)
	if !errors.Is(err, ErrBufferFull) {
		t.Fatalf("third publish error = %v, want ErrBufferFull", err)
	}

	close(release)
}

func TestAsyncDropOldestKeepsLatest(t *testing.T) {
	bus := New()

	var (
		mu      sync.Mutex
		values  []int
		wg      sync.WaitGroup
		started = make(chan struct{}, 1)
		release = make(chan struct{})
	)

	wg.Add(2)
	unsubscribe, err := Subscribe(bus, func(ctx context.Context, event Event[int]) error {
		select {
		case started <- struct{}{}:
		default:
		}

		if event.Value == 1 {
			<-release
		}

		mu.Lock()
		values = append(values, event.Value)
		mu.Unlock()
		wg.Done()
		return nil
	}, Async(), WithBuffer(1), WithOverflow(OverflowDropOldest))
	if err != nil {
		t.Fatalf("Subscribe() error = %v", err)
	}
	defer unsubscribe()

	if err := Publish(context.Background(), bus, 1); err != nil {
		t.Fatalf("first publish error = %v", err)
	}
	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for handler start")
	}

	if err := Publish(context.Background(), bus, 2); err != nil {
		t.Fatalf("second publish error = %v", err)
	}

	err = Publish(context.Background(), bus, 3)
	if !errors.Is(err, ErrDropped) {
		t.Fatalf("third publish error = %v, want ErrDropped", err)
	}

	close(release)

	done := make(chan struct{})
	go func() {
		wg.Wait()
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for async handler")
	}

	if !slices.Equal(values, []int{1, 3}) {
		t.Fatalf("got %v, want [1 3]", values)
	}
}

func TestSubscribeTopicWildcard(t *testing.T) {
	bus := New()

	var got []string
	var mu sync.Mutex

	unsubscribe, err := SubscribeTopic(bus, "orders.>", func(ctx context.Context, event Event[string]) error {
		mu.Lock()
		defer mu.Unlock()
		got = append(got, event.Topic)
		return nil
	})
	if err != nil {
		t.Fatalf("SubscribeTopic() error = %v", err)
	}
	defer unsubscribe()

	if err := Publish(context.Background(), bus, "created", WithTopic("orders.created")); err != nil {
		t.Fatalf("Publish(orders.created) error = %v", err)
	}
	if err := Publish(context.Background(), bus, "archived", WithTopic("orders.eu.archived")); err != nil {
		t.Fatalf("Publish(orders.eu.archived) error = %v", err)
	}
	if err := Publish(context.Background(), bus, "ignored", WithTopic("payments.created")); err != nil {
		t.Fatalf("Publish(payments.created) error = %v", err)
	}

	if !reflect.DeepEqual(got, []string{"orders.created", "orders.eu.archived"}) {
		t.Fatalf("got %v, want matching order topics", got)
	}
}

func TestUnsubscribeStopsNewMessages(t *testing.T) {
	bus := New()

	var calls atomic.Int64
	unsubscribe, err := Subscribe(bus, func(ctx context.Context, event Event[int]) error {
		calls.Add(1)
		return nil
	})
	if err != nil {
		t.Fatalf("Subscribe() error = %v", err)
	}

	if err := Publish(context.Background(), bus, 1); err != nil {
		t.Fatalf("first publish error = %v", err)
	}

	unsubscribe()

	if err := Publish(context.Background(), bus, 2); err != nil {
		t.Fatalf("second publish error = %v", err)
	}

	if calls.Load() != 1 {
		t.Fatalf("handler called %d times, want 1", calls.Load())
	}
}

func TestUnsubscribeIsIdempotentAndAllowsInFlightAsync(t *testing.T) {
	bus := New()

	started := make(chan struct{}, 1)
	release := make(chan struct{})
	var calls atomic.Int64

	unsubscribe, err := Subscribe(bus, func(ctx context.Context, event Event[int]) error {
		calls.Add(1)
		select {
		case started <- struct{}{}:
		default:
		}
		<-release
		return nil
	}, Async(), WithBuffer(1))
	if err != nil {
		t.Fatalf("Subscribe() error = %v", err)
	}

	if err := Publish(context.Background(), bus, 1); err != nil {
		t.Fatalf("first publish error = %v", err)
	}

	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for handler start")
	}

	unsubscribe()
	unsubscribe()

	if err := Publish(context.Background(), bus, 2); err != nil {
		t.Fatalf("publish after unsubscribe error = %v", err)
	}

	close(release)
	time.Sleep(50 * time.Millisecond)

	if calls.Load() != 1 {
		t.Fatalf("handler called %d times, want 1", calls.Load())
	}
}

func TestCloseRejectsNewPublishAndDrainsAsync(t *testing.T) {
	bus := New()

	var processed atomic.Int64
	unsubscribe, err := Subscribe(bus, func(ctx context.Context, event Event[int]) error {
		time.Sleep(10 * time.Millisecond)
		processed.Add(1)
		return nil
	}, Async(), WithBuffer(4))
	if err != nil {
		t.Fatalf("Subscribe() error = %v", err)
	}
	defer unsubscribe()

	for i := range 3 {
		if err := Publish(context.Background(), bus, i); err != nil {
			t.Fatalf("Publish(%d) error = %v", i, err)
		}
	}

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	if err := bus.Close(ctx); err != nil {
		t.Fatalf("Close() error = %v", err)
	}

	if processed.Load() != 3 {
		t.Fatalf("processed %d events, want 3", processed.Load())
	}

	err = Publish(context.Background(), bus, 99)
	if !errors.Is(err, ErrClosed) {
		t.Fatalf("publish after close error = %v, want ErrClosed", err)
	}
}

func TestCloseWaitsForInFlightPublishBeforeStoppingSubscriptions(t *testing.T) {
	bus := New()

	predicateEntered := make(chan struct{}, 1)
	releasePredicate := make(chan struct{})
	delivered := make(chan int, 1)

	unsubscribe, err := SubscribeMatch(bus, func(event Event[int]) bool {
		select {
		case predicateEntered <- struct{}{}:
		default:
		}
		<-releasePredicate
		return true
	}, func(ctx context.Context, event Event[int]) error {
		delivered <- event.Value
		return nil
	})
	if err != nil {
		t.Fatalf("SubscribeMatch() error = %v", err)
	}
	defer unsubscribe()

	publishDone := make(chan error, 1)
	go func() {
		publishDone <- Publish(context.Background(), bus, 42)
	}()

	select {
	case <-predicateEntered:
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for in-flight publish to reach predicate")
	}

	closeDone := make(chan error, 1)
	go func() {
		closeDone <- bus.Close(context.Background())
	}()

	time.Sleep(20 * time.Millisecond)
	close(releasePredicate)

	select {
	case value := <-delivered:
		if value != 42 {
			t.Fatalf("delivered value = %d, want 42", value)
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for in-flight publish delivery")
	}

	select {
	case err := <-publishDone:
		if err != nil {
			t.Fatalf("Publish() error = %v, want nil", err)
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for publish completion")
	}

	select {
	case err := <-closeDone:
		if err != nil {
			t.Fatalf("Close() error = %v, want nil", err)
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for close completion")
	}
}

func TestAsyncSubscribeAfterCloseDoesNotLeakWorkers(t *testing.T) {
	bus := New()
	if err := bus.Close(context.Background()); err != nil {
		t.Fatalf("Close() error = %v", err)
	}

	before := runtime.NumGoroutine()

	for range 20 {
		unsubscribe, err := Subscribe(bus, func(ctx context.Context, event Event[int]) error {
			return nil
		}, Async(), WithParallelism(3), WithBuffer(8))
		if !errors.Is(err, ErrClosed) {
			t.Fatalf("Subscribe() error = %v, want ErrClosed", err)
		}
		if unsubscribe != nil {
			t.Fatal("unsubscribe = non-nil, want nil")
		}
	}

	time.Sleep(50 * time.Millisecond)
	runtime.GC()
	time.Sleep(50 * time.Millisecond)

	after := runtime.NumGoroutine()
	if delta := after - before; delta > 2 {
		t.Fatalf("goroutines leaked after failed async subscribe attempts: before=%d after=%d", before, after)
	}
}

func TestCloseIsIdempotent(t *testing.T) {
	bus := New()

	if err := bus.Close(context.Background()); err != nil {
		t.Fatalf("first Close() error = %v", err)
	}
	if err := bus.Close(context.Background()); err != nil {
		t.Fatalf("second Close() error = %v", err)
	}
}

func TestCloseTimeoutReportsIncompleteDrain(t *testing.T) {
	bus := New()

	started := make(chan struct{}, 1)
	release := make(chan struct{})

	unsubscribe, err := Subscribe(bus, func(ctx context.Context, event Event[int]) error {
		select {
		case started <- struct{}{}:
		default:
		}
		<-release
		return nil
	}, Async(), WithBuffer(1))
	if err != nil {
		t.Fatalf("Subscribe() error = %v", err)
	}
	defer unsubscribe()

	if err := Publish(context.Background(), bus, 1); err != nil {
		t.Fatalf("Publish() error = %v", err)
	}

	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for async handler start")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()

	err = bus.Close(ctx)
	if !errors.Is(err, ErrCloseIncomplete) {
		t.Fatalf("Close() error = %v, want ErrCloseIncomplete", err)
	}
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("Close() error = %v, want context deadline exceeded", err)
	}

	close(release)

	if err := bus.Close(context.Background()); err != nil {
		t.Fatalf("second Close() error = %v", err)
	}
}

func TestPublishHooks(t *testing.T) {
	startCh := make(chan PublishStart, 1)
	doneCh := make(chan PublishDone, 1)

	bus := New(WithHooks(Hooks{
		OnPublishStart: func(info PublishStart) {
			startCh <- info
		},
		OnPublishDone: func(info PublishDone) {
			doneCh <- info
		},
	}))

	unsubscribe, err := SubscribeTopic(bus, "orders.>", func(ctx context.Context, event Event[string]) error {
		return nil
	})
	if err != nil {
		t.Fatalf("SubscribeTopic() error = %v", err)
	}
	defer unsubscribe()

	if err := Publish(context.Background(), bus, "created", WithTopic("orders.created"), WithKey("k1")); err != nil {
		t.Fatalf("Publish() error = %v", err)
	}

	select {
	case info := <-startCh:
		if info.Topic != "orders.created" || info.Key != "k1" {
			t.Fatalf("unexpected start hook info: %+v", info)
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for publish start hook")
	}

	select {
	case info := <-doneCh:
		if info.MatchedSubscribers != 1 {
			t.Fatalf("done hook matched %d subscribers, want 1", info.MatchedSubscribers)
		}
		if info.DeliveredSubscribers != 1 {
			t.Fatalf("done hook delivered %d subscribers, want 1", info.DeliveredSubscribers)
		}
		if info.Err != nil {
			t.Fatalf("done hook error = %v, want nil", info.Err)
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for publish done hook")
	}
}

func TestPublishDoneSeparatesMatchedAndDeliveredSubscribers(t *testing.T) {
	doneCh := make(chan PublishDone, 1)
	bus := New(WithHooks(Hooks{
		OnPublishDone: func(info PublishDone) {
			doneCh <- info
		},
	}))

	var called atomic.Int64
	var unsubscribe func()

	var err error
	unsubscribe, err = SubscribeMatch(bus, func(event Event[int]) bool {
		unsubscribe()
		return true
	}, func(ctx context.Context, event Event[int]) error {
		called.Add(1)
		return nil
	})
	if err != nil {
		t.Fatalf("SubscribeMatch() error = %v", err)
	}

	if err := Publish(context.Background(), bus, 1); err != nil {
		t.Fatalf("Publish() error = %v", err)
	}

	select {
	case info := <-doneCh:
		if info.MatchedSubscribers != 1 {
			t.Fatalf("MatchedSubscribers = %d, want 1", info.MatchedSubscribers)
		}
		if info.DeliveredSubscribers != 0 {
			t.Fatalf("DeliveredSubscribers = %d, want 0", info.DeliveredSubscribers)
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for publish done hook")
	}

	if called.Load() != 0 {
		t.Fatalf("handler called %d times, want 0", called.Load())
	}
}

func TestHookPanicsAreReportedWithoutBreakingPublish(t *testing.T) {
	hookPanicCh := make(chan HookPanic, 1)
	bus := New(WithHooks(Hooks{
		OnPublishDone: func(PublishDone) {
			panic("hook boom")
		},
		OnHookPanic: func(info HookPanic) {
			hookPanicCh <- info
			panic("reporter boom")
		},
	}))

	unsubscribe, err := Subscribe(bus, func(ctx context.Context, event Event[int]) error {
		return nil
	})
	if err != nil {
		t.Fatalf("Subscribe() error = %v", err)
	}
	defer unsubscribe()

	if err := Publish(context.Background(), bus, 1); err != nil {
		t.Fatalf("Publish() error = %v, want nil", err)
	}

	select {
	case info := <-hookPanicCh:
		if info.Hook != "OnPublishDone" {
			t.Fatalf("Hook = %q, want OnPublishDone", info.Hook)
		}
		if info.Value != "hook boom" {
			t.Fatalf("Value = %v, want hook boom", info.Value)
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for hook panic report")
	}
}

func TestAsyncHandlerErrorVisibleViaHook(t *testing.T) {
	wantErr := errors.New("handler failed")
	errorCh := make(chan HandlerError, 1)

	bus := New(WithHooks(Hooks{
		OnHandlerError: func(info HandlerError) {
			errorCh <- info
		},
	}))

	unsubscribe, err := Subscribe(bus, func(ctx context.Context, event Event[int]) error {
		return wantErr
	}, Async(), WithBuffer(1))
	if err != nil {
		t.Fatalf("Subscribe() error = %v", err)
	}
	defer unsubscribe()

	if err := Publish(context.Background(), bus, 1); err != nil {
		t.Fatalf("Publish() error = %v", err)
	}

	select {
	case info := <-errorCh:
		if !info.Async {
			t.Fatalf("hook Async = false, want true")
		}
		if !errors.Is(info.Err, wantErr) {
			t.Fatalf("hook err = %v, want %v", info.Err, wantErr)
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for async handler error hook")
	}
}

func TestSyncHandlerPanicReturnedAndHooked(t *testing.T) {
	panicCh := make(chan HandlerPanic, 1)

	bus := New(WithHooks(Hooks{
		OnHandlerPanic: func(info HandlerPanic) {
			panicCh <- info
		},
	}))

	unsubscribe, err := Subscribe(bus, func(ctx context.Context, event Event[int]) error {
		panic("boom")
	})
	if err != nil {
		t.Fatalf("Subscribe() error = %v", err)
	}
	defer unsubscribe()

	err = Publish(context.Background(), bus, 1)
	if !errors.Is(err, ErrHandlerPanic) {
		t.Fatalf("Publish() error = %v, want ErrHandlerPanic", err)
	}

	select {
	case info := <-panicCh:
		if info.Async {
			t.Fatalf("hook Async = true, want false")
		}
		if info.Value != "boom" {
			t.Fatalf("hook panic value = %v, want boom", info.Value)
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for handler panic hook")
	}
}

func TestDropHookFiresOnDropNewest(t *testing.T) {
	dropCh := make(chan DroppedEvent, 1)
	started := make(chan struct{}, 1)
	release := make(chan struct{})

	bus := New(WithHooks(Hooks{
		OnEventDropped: func(info DroppedEvent) {
			dropCh <- info
		},
	}))

	unsubscribe, err := Subscribe(bus, func(ctx context.Context, event Event[int]) error {
		select {
		case started <- struct{}{}:
		default:
		}
		<-release
		return nil
	}, Async(), WithBuffer(1), WithOverflow(OverflowDropNewest))
	if err != nil {
		t.Fatalf("Subscribe() error = %v", err)
	}
	defer unsubscribe()

	if err := Publish(context.Background(), bus, 1); err != nil {
		t.Fatalf("first publish error = %v", err)
	}
	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for handler start")
	}

	if err := Publish(context.Background(), bus, 2); err != nil {
		t.Fatalf("second publish error = %v", err)
	}

	err = Publish(context.Background(), bus, 3)
	if !errors.Is(err, ErrDropped) {
		t.Fatalf("third publish error = %v, want ErrDropped", err)
	}

	select {
	case info := <-dropCh:
		if info.Policy != OverflowDropNewest {
			t.Fatalf("drop hook policy = %v, want OverflowDropNewest", info.Policy)
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for drop hook")
	}

	close(release)
}
