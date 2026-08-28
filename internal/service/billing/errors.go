package billing

import "errors"

var (
	ErrNilStorage         = errors.New("billing storage is nil")
	ErrInvalidBillingDate = errors.New("billing date is invalid")
)
