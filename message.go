package busen

// Event is the typed value delivered to handlers.
type Event[T any] struct {
	Topic   string
	Key     string
	Value   T
	Headers map[string]string
}

type envelope struct {
	topic   string
	key     string
	value   any
	headers map[string]string
}

func typedEvent[T any](e envelope) Event[T] {
	return Event[T]{
		Topic:   e.topic,
		Key:     e.key,
		Value:   e.value.(T),
		Headers: cloneHeaders(e.headers),
	}
}

func cloneHeaders(headers map[string]string) map[string]string {
	if len(headers) == 0 {
		return nil
	}

	cloned := make(map[string]string, len(headers))
	for k, v := range headers {
		cloned[k] = v
	}
	return cloned
}
