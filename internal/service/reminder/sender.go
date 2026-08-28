package reminder

import (
	"context"
	"errors"
)

// Sender is a narrow transport boundary for reminder delivery.
type Sender interface {
	SendReminder(context.Context, int64, string) (int, error)
}

type DeliveryErrorCode string

const (
	DeliveryErrorFailed  DeliveryErrorCode = "delivery_failed"
	DeliveryErrorUnknown DeliveryErrorCode = "delivery_state_unknown"
	DeliveryErrorOffline DeliveryErrorCode = "unreachable"
)

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
