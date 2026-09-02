package domain

import (
	"errors"
	"math"
	"testing"
)

func TestAddAmounts(t *testing.T) {
	tests := []struct {
		name  string
		left  AmountMinor
		right AmountMinor
		want  AmountMinor
		err   error
	}{
		{
			name:  "positive values",
			left:  10_000,
			right: 5_000,
			want:  15_000,
		},
		{
			name:  "positive and negative values",
			left:  10_000,
			right: -7_000,
			want:  3_000,
		},
		{
			name:  "zero result",
			left:  5_000,
			right: -5_000,
			want:  0,
		},
		{
			name:  "positive overflow",
			left:  AmountMinor(math.MaxInt64),
			right: 1,
			err:   ErrAmountOverflow,
		},
		{
			name:  "negative overflow",
			left:  AmountMinor(math.MinInt64),
			right: -1,
			err:   ErrAmountOverflow,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, err := AddAmounts(test.left, test.right)
			if !errors.Is(err, test.err) {
				t.Fatalf("AddAmounts() error = %v, want %v", err, test.err)
			}
			if got != test.want {
				t.Fatalf("AddAmounts() = %d, want %d", got, test.want)
			}
		})
	}
}

func TestAmountMinorInt64(t *testing.T) {
	amount := AmountMinor(-10_050)
	if got := amount.Int64(); got != -10_050 {
		t.Fatalf("Int64() = %d, want -10050", got)
	}
}
