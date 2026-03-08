package busen

import (
	"context"
	"reflect"
)

// MetadataBuilder builds optional structured metadata for publish envelopes.
type MetadataBuilder func(PublishMetadataInput) map[string]string

// PublishMetadataInput is passed to MetadataBuilder.
type PublishMetadataInput struct {
	Context   context.Context
	EventType reflect.Type
	Topic     string
	Key       string
	Headers   map[string]string
	Value     any
}
