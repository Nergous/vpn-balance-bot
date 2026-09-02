package domain

import (
	"errors"
	"math"
)

var ErrAmountOverflow = errors.New("amount overflow")

type AmountMinor int64

func (a AmountMinor) Int64() int64 {
	return int64(a)
}

func AddAmounts(a, b AmountMinor) (AmountMinor, error) {
	left := int64(a)
	right := int64(b)

	if right > 0 && left > math.MaxInt64-right {
		return 0, ErrAmountOverflow
	}

	if right < 0 && left < math.MinInt64-right {
		return 0, ErrAmountOverflow
	}

	return AmountMinor(left + right), nil
}
