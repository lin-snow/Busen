package busen

// Event is the typed value delivered to handlers.
type Event[T any] struct {
	// Topic carries optional routing metadata supplied at publish time.
	Topic string
	// Key carries the optional ordering key supplied at publish time.
	Key string
	// Value is the typed event payload.
	Value T
	// Headers contains a shallow copy of publish headers visible to handlers.
	Headers map[string]string
	// Meta contains structured envelope metadata visible to handlers.
	Meta map[string]string
}

type envelope struct {
	topic   string
	key     string
	value   any
	headers map[string]string
	meta    map[string]string
}

func typedEvent[T any](e envelope) Event[T] {
	return Event[T]{
		Topic:   e.topic,
		Key:     e.key,
		Value:   e.value.(T),
		Headers: cloneHeaders(e.headers),
		Meta:    cloneHeaders(e.meta),
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
