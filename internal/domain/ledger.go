package domain

import "time"

type LedgerKind string

const (
	LedgerKindOpeningBalance     LedgerKind = "opening_balance"
	LedgerKindPayment            LedgerKind = "payment"
	LedgerKindSubscriptionCharge LedgerKind = "subscription_charge"
	LedgerKindAdjustment         LedgerKind = "adjustment"
	LedgerKindReversal           LedgerKind = "reversal"
)

// IsValid reports whether the ledger kind is supported by the MVP.
func (k LedgerKind) IsValid() bool {
	switch k {
	case LedgerKindOpeningBalance,
		LedgerKindPayment,
		LedgerKindSubscriptionCharge,
		LedgerKindAdjustment,
		LedgerKindReversal:
		return true
	default:
		return false
	}
}

// LedgerEntry is an immutable signed financial operation.
type LedgerEntry struct {
	ID                  int64
	UserID              UserID
	Kind                LedgerKind
	AmountMinor         AmountMinor
	OccurredAt          time.Time
	BillingPeriodOn     *Date
	ReversesEntryID     *int64
	CreatedByTelegramID *int64
	Note                *string
	CreatedAt           time.Time
}

// CalculateBalance returns the sum of signed ledger amounts.
// Callers must pass entries belonging to one user.
func CalculateBalance(entries []LedgerEntry) (AmountMinor, error) {
	var balance AmountMinor

	for _, entry := range entries {
		nextBalance, err := AddAmounts(balance, entry.AmountMinor)
		if err != nil {
			return 0, err
		}

		balance = nextBalance
	}

	return balance, nil
}
