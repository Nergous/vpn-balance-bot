package reminder

import (
	"context"
	"errors"
)

// Sender is a narrow transport boundary for reminder delivery.
type Sender interface {
	SendReminder(context.Context, int64, string) (int, error)
}

// DeliveryErrorCode classifies a failed or uncertain reminder delivery.
type DeliveryErrorCode string

const (
	// DeliveryErrorFailed indicates a definite delivery failure.
	DeliveryErrorFailed DeliveryErrorCode = "delivery_failed"
	// DeliveryErrorRetryable indicates a definite pre-send transient failure.
	DeliveryErrorRetryable DeliveryErrorCode = "delivery_retryable"
	// DeliveryErrorUnknown indicates that delivery outcome cannot be determined.
	DeliveryErrorUnknown DeliveryErrorCode = "delivery_state_unknown"
	// DeliveryErrorOffline indicates that the recipient is unreachable.
	DeliveryErrorOffline DeliveryErrorCode = "unreachable"
)

// ErrRetryableDelivery marks a delivery error that scheduler may retry safely.
var ErrRetryableDelivery error = retryableDeliveryError{}

type retryableDeliveryError struct{}

func (retryableDeliveryError) Error() string { return "reminder delivery is retryable" }

func (retryableDeliveryError) Retryable() bool { return true }

// ErrorClassifier is optionally implemented by a transport adapter. It keeps
// provider-specific error types out of reminder use cases.
type ErrorClassifier interface {
	ClassifyReminderError(error) DeliveryErrorCode
}

func classifyDeliveryError(sender Sender, err error) DeliveryErrorCode {
	if classifier, ok := sender.(ErrorClassifier); ok {
		if code := classifier.ClassifyReminderError(err); code != "" {
			return code
		}
	}

	if errors.Is(err, context.DeadlineExceeded) || errors.Is(err, context.Canceled) {
		return DeliveryErrorUnknown
	}

	return DeliveryErrorFailed
}
