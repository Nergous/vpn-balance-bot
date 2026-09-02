package domain

import (
	"errors"
	"math"
	"testing"
)

func TestCalculateBalance(t *testing.T) {
	tests := []struct {
		name    string
		entries []LedgerEntry
		want    AmountMinor
		err     error
	}{
		{
			name: "empty ledger",
			want: 0,
		},
		{
			name: "positive balance",
			entries: []LedgerEntry{
				{AmountMinor: 10_000},
				{AmountMinor: 5_000},
			},
			want: 15_000,
		},
		{
			name: "negative balance",
			entries: []LedgerEntry{
				{AmountMinor: 5_000},
				{AmountMinor: -8_000},
			},
			want: -3_000,
		},
		{
			name: "zero balance",
			entries: []LedgerEntry{
				{AmountMinor: 5_000},
				{AmountMinor: -5_000},
			},
			want: 0,
		},
		{
			name: "overflow",
			entries: []LedgerEntry{
				{AmountMinor: AmountMinor(math.MaxInt64)},
				{AmountMinor: 1},
			},
			err: ErrAmountOverflow,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, err := CalculateBalance(test.entries)
			if !errors.Is(err, test.err) {
				t.Fatalf("CalculateBalance() error = %v, want %v", err, test.err)
			}
			if got != test.want {
				t.Fatalf("CalculateBalance() = %d, want %d", got, test.want)
			}
		})
	}
}

func TestLedgerKindIsValid(t *testing.T) {
	for _, kind := range []LedgerKind{
		LedgerKindOpeningBalance,
		LedgerKindPayment,
		LedgerKindSubscriptionCharge,
		LedgerKindAdjustment,
		LedgerKindReversal,
	} {
		if !kind.IsValid() {
			t.Errorf("LedgerKind %q was rejected", kind)
		}
	}

	if LedgerKind("unknown").IsValid() {
		t.Error("unknown ledger kind was accepted")
	}
}
