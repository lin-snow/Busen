package busen

import "errors"

var (
	// ErrClosed indicates that the bus no longer accepts new publishes or subscriptions.
	ErrClosed = errors.New("busen: bus closed")

	// ErrHandlerNil indicates that a nil handler was passed to Subscribe.
	ErrHandlerNil = errors.New("busen: handler is nil")

	// ErrBufferFull indicates that an asynchronous subscriber queue is full.
	ErrBufferFull = errors.New("busen: subscriber buffer full")

	// ErrDropped indicates that at least one event was dropped due to backpressure.
	ErrDropped = errors.New("busen: event dropped")

	// ErrInvalidPattern indicates that a topic pattern is malformed.
	ErrInvalidPattern = errors.New("busen: invalid topic pattern")

	// ErrInvalidOption indicates that an option value is not valid.
	ErrInvalidOption = errors.New("busen: invalid option")

	// ErrHandlerPanic indicates that a handler panicked while processing an event.
	ErrHandlerPanic = errors.New("busen: handler panic")

	// ErrCloseIncomplete indicates that Close stopped new work but did not finish
	// draining all in-flight work before the provided context ended.
	ErrCloseIncomplete = errors.New("busen: close incomplete")
)
