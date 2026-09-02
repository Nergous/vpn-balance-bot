package billing

import "errors"

var (
	ErrNilStorage                = errors.New("billing storage is nil")
	ErrInvalidBillingDate        = errors.New("billing date is invalid")
	ErrCatchUpLimitReached error = catchUpLimitError{}
)

type catchUpLimitError struct{}

func (catchUpLimitError) Error() string { return "billing catch-up limit reached" }

func (catchUpLimitError) Retryable() bool { return true }
