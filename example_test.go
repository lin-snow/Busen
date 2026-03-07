package busen_test

import (
	"context"
	"fmt"
	"log"

	"github.com/lin-snow/Busen"
)

func Example() {
	type UserCreated struct {
		Email string
	}

	b := busen.New()

	unsubscribe, err := busen.Subscribe(b, func(_ context.Context, event busen.Event[UserCreated]) error {
		fmt.Println("welcome", event.Value.Email)
		return nil
	})
	if err != nil {
		log.Fatal(err)
	}
	defer unsubscribe()

	if err := busen.Publish(context.Background(), b, UserCreated{Email: "hello@example.com"}); err != nil {
		log.Fatal(err)
	}

	if err := b.Close(context.Background()); err != nil {
		log.Fatal(err)
	}

	// Output:
	// welcome hello@example.com
}

func ExampleSubscribeTopic() {
	b := busen.New()

	unsubscribe, err := busen.SubscribeTopic(b, "orders.>", func(_ context.Context, event busen.Event[string]) error {
		fmt.Printf("%s=%s\n", event.Topic, event.Value)
		return nil
	})
	if err != nil {
		log.Fatal(err)
	}
	defer unsubscribe()

	if err := busen.Publish(context.Background(), b, "created", busen.WithTopic("orders.eu.created")); err != nil {
		log.Fatal(err)
	}

	if err := b.Close(context.Background()); err != nil {
		log.Fatal(err)
	}

	// Output:
	// orders.eu.created=created
}

func ExampleSubscribeTopics() {
	b := busen.New()

	unsubscribe, err := busen.SubscribeTopics(b, []string{"orders.created", "orders.updated"}, func(_ context.Context, event busen.Event[string]) error {
		fmt.Printf("%s=%s\n", event.Topic, event.Value)
		return nil
	})
	if err != nil {
		log.Fatal(err)
	}
	defer unsubscribe()

	if err := busen.Publish(context.Background(), b, "created", busen.WithTopic("orders.created")); err != nil {
		log.Fatal(err)
	}
	if err := busen.Publish(context.Background(), b, "updated", busen.WithTopic("orders.updated")); err != nil {
		log.Fatal(err)
	}

	if err := b.Close(context.Background()); err != nil {
		log.Fatal(err)
	}

	// Output:
	// orders.created=created
	// orders.updated=updated
}

func ExampleAsync() {
	type JobQueued struct {
		ID string
	}

	b := busen.New()
	done := make(chan struct{})

	unsubscribe, err := busen.Subscribe(b, func(_ context.Context, event busen.Event[JobQueued]) error {
		fmt.Println("processed", event.Value.ID)
		close(done)
		return nil
	}, busen.Async(), busen.WithBuffer(1))
	if err != nil {
		log.Fatal(err)
	}
	defer unsubscribe()

	if err := busen.Publish(context.Background(), b, JobQueued{ID: "job-42"}); err != nil {
		log.Fatal(err)
	}

	<-done

	if err := b.Close(context.Background()); err != nil {
		log.Fatal(err)
	}

	// Output:
	// processed job-42
}

func ExampleWithKey() {
	type UserCreated struct {
		ID string
	}

	b := busen.New()
	done := make(chan struct{}, 2)

	unsubscribe, err := busen.Subscribe(b, func(_ context.Context, event busen.Event[UserCreated]) error {
		fmt.Printf("%s:%s\n", event.Key, event.Value.ID)
		done <- struct{}{}
		return nil
	}, busen.Async(), busen.WithParallelism(2), busen.WithBuffer(4))
	if err != nil {
		log.Fatal(err)
	}
	defer unsubscribe()

	if err := busen.Publish(context.Background(), b, UserCreated{ID: "1"}, busen.WithKey("tenant-a")); err != nil {
		log.Fatal(err)
	}
	if err := busen.Publish(context.Background(), b, UserCreated{ID: "2"}, busen.WithKey("tenant-a")); err != nil {
		log.Fatal(err)
	}

	<-done
	<-done

	if err := b.Close(context.Background()); err != nil {
		log.Fatal(err)
	}

	// Output:
	// tenant-a:1
	// tenant-a:2
}

func ExampleBus_Use() {
	type AuditEvent struct {
		Action string
	}

	b := busen.New()
	if err := b.Use(func(next busen.Next) busen.Next {
		return func(ctx context.Context, dispatch busen.Dispatch) error {
			if dispatch.Headers == nil {
				dispatch.Headers = make(map[string]string, 1)
			}
			dispatch.Headers["source"] = "middleware"
			return next(ctx, dispatch)
		}
	}); err != nil {
		log.Fatal(err)
	}

	unsubscribe, err := busen.Subscribe(b, func(_ context.Context, event busen.Event[AuditEvent]) error {
		fmt.Printf("%s from %s\n", event.Value.Action, event.Headers["source"])
		return nil
	})
	if err != nil {
		log.Fatal(err)
	}
	defer unsubscribe()

	if err := busen.Publish(
		context.Background(),
		b,
		AuditEvent{Action: "saved"},
		busen.WithHeaders(map[string]string{"request-id": "req-1"}),
	); err != nil {
		log.Fatal(err)
	}

	if err := b.Close(context.Background()); err != nil {
		log.Fatal(err)
	}

	// Output:
	// saved from middleware
}

func ExampleWithHooks() {
	type UserCreated struct {
		ID string
	}

	b := busen.New(busen.WithHooks(busen.Hooks{
		OnPublishDone: func(info busen.PublishDone) {
			fmt.Printf("matched=%d delivered=%d\n", info.MatchedSubscribers, info.DeliveredSubscribers)
		},
	}))

	unsubscribe, err := busen.Subscribe(b, func(_ context.Context, event busen.Event[UserCreated]) error {
		fmt.Println("handled", event.Value.ID)
		return nil
	})
	if err != nil {
		log.Fatal(err)
	}
	defer unsubscribe()

	if err := busen.Publish(context.Background(), b, UserCreated{ID: "u-1"}); err != nil {
		log.Fatal(err)
	}

	if err := b.Close(context.Background()); err != nil {
		log.Fatal(err)
	}

	// Output:
	// handled u-1
	// matched=1 delivered=1
}
