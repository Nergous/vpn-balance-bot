package account

import (
	"context"
	"errors"
	"math"
	"testing"
	"time"

	"github.com/Nergous/vpn-balance-bot/internal/domain"
)

func TestAddPaymentRequiresPositiveAmount(t *testing.T) {
	service := newService(&fakeStorage{}, time.Hour, time.Now)
	for _, amount := range []domain.AmountMinor{0, -1} {
		_, err := service.AddPayment(context.Background(), AddPaymentParams{AmountMinor: amount, AdminTelegramID: 1})
		if !errors.Is(err, ErrInvalidPaymentAmount) {
			t.Fatalf("AddPayment(%d) error = %v, want %v", amount, err, ErrInvalidPaymentAmount)
		}
	}
}

func TestAddPaymentPreservesExplicitOccurrenceTime(t *testing.T) {
	createdAt := time.Date(2026, time.September, 3, 10, 0, 0, 0, time.UTC)
	occurredAt := time.Date(2026, time.September, 2, 18, 30, 0, 0, time.FixedZone("payment", 3*60*60))
	var got domain.LedgerEntry
	storage := &fakeStorage{createLedgerEntry: func(_ context.Context, entry domain.LedgerEntry) (domain.LedgerEntry, error) {
		got = entry
		return entry, nil
	}}
	service := newService(storage, time.Hour, func() time.Time { return createdAt })

	if _, err := service.AddPayment(context.Background(), AddPaymentParams{
		UserID: 1, AmountMinor: 100, AdminTelegramID: 7, OccurredAt: &occurredAt,
	}); err != nil {
		t.Fatal(err)
	}
	if !got.OccurredAt.Equal(occurredAt) || got.OccurredAt.Location() != time.UTC {
		t.Fatalf("OccurredAt = %v", got.OccurredAt)
	}
	if !got.CreatedAt.Equal(createdAt) {
		t.Fatalf("CreatedAt = %v", got.CreatedAt)
	}
}

func TestManualLedgerOperationsBuildEntries(t *testing.T) {
	now := time.Date(2026, time.August, 28, 10, 0, 0, 0, time.UTC)
	var entries []domain.LedgerEntry
	storage := &fakeStorage{
		createLedgerEntry: func(_ context.Context, entry domain.LedgerEntry) (domain.LedgerEntry, error) {
			entries = append(entries, entry)
			return entry, nil
		},
	}
	service := newService(storage, time.Hour, func() time.Time { return now })

	if _, err := service.AddOpeningBalance(context.Background(), AddOpeningBalanceParams{UserID: 1, AmountMinor: -500, AdminTelegramID: 7}); err != nil {
		t.Fatal(err)
	}
	if _, err := service.AddPayment(context.Background(), AddPaymentParams{UserID: 1, AmountMinor: 1000, AdminTelegramID: 7}); err != nil {
		t.Fatal(err)
	}
	if _, err := service.AddAdjustment(context.Background(), AddAdjustmentParams{UserID: 1, AmountMinor: -100, AdminTelegramID: 7, Note: "  correction  "}); err != nil {
		t.Fatal(err)
	}
	if len(entries) != 3 {
		t.Fatalf("created entries = %d, want 3", len(entries))
	}
	if entries[0].Kind != domain.LedgerKindOpeningBalance || entries[0].AmountMinor != -500 {
		t.Fatalf("opening entry = %#v", entries[0])
	}
	if entries[1].Kind != domain.LedgerKindPayment || entries[1].AmountMinor != 1000 {
		t.Fatalf("payment entry = %#v", entries[1])
	}
	if entries[2].Kind != domain.LedgerKindAdjustment || entries[2].Note == nil || *entries[2].Note != "correction" {
		t.Fatalf("adjustment entry = %#v", entries[2])
	}
	for _, entry := range entries {
		if entry.CreatedByTelegramID == nil || *entry.CreatedByTelegramID != 7 || !entry.OccurredAt.Equal(now) || !entry.CreatedAt.Equal(now) {
			t.Fatalf("entry audit fields = %#v", entry)
		}
	}
}

func TestSignedManualLedgerOperationsRejectMinInt64BeforeStorage(t *testing.T) {
	storageCalled := false
	storage := &fakeStorage{
		createLedgerEntry: func(_ context.Context, entry domain.LedgerEntry) (domain.LedgerEntry, error) {
			storageCalled = true
			return entry, nil
		},
	}
	service := newService(storage, time.Hour, time.Now)

	tests := []struct {
		name string
		call func() error
	}{
		{
			name: "opening balance",
			call: func() error {
				_, err := service.AddOpeningBalance(context.Background(), AddOpeningBalanceParams{
					UserID: 1, AmountMinor: domain.AmountMinor(math.MinInt64), AdminTelegramID: 7,
				})
				return err
			},
		},
		{
			name: "adjustment",
			call: func() error {
				_, err := service.AddAdjustment(context.Background(), AddAdjustmentParams{
					UserID: 1, AmountMinor: domain.AmountMinor(math.MinInt64), AdminTelegramID: 7, Note: "correction",
				})
				return err
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			storageCalled = false
			if err := test.call(); !errors.Is(err, domain.ErrAmountOverflow) {
				t.Fatalf("error = %v, want %v", err, domain.ErrAmountOverflow)
			}
			if storageCalled {
				t.Fatal("storage called after MinInt64 validation failure")
			}
		})
	}
}

func TestLastUnreversedPaymentForwardsExactContract(t *testing.T) {
	want := domain.LedgerEntry{ID: 12, UserID: 7, Kind: domain.LedgerKindPayment}
	storage := &fakeStorage{
		lastUnreversedPayment: func(_ context.Context, userID domain.UserID) (domain.LedgerEntry, bool, error) {
			if userID != 7 {
				t.Fatalf("LastUnreversedPayment userID = %d, want 7", userID)
			}
			return want, true, nil
		},
	}

	got, found, err := newService(storage, time.Hour, time.Now).LastUnreversedPayment(context.Background(), 7)
	if err != nil || !found || got != want {
		t.Fatalf("LastUnreversedPayment() = %#v, %t, %v; want %#v, true, nil", got, found, err, want)
	}
}

func TestLedgerValidationAndReversalForwarding(t *testing.T) {
	now := time.Date(2026, time.August, 28, 10, 0, 0, 0, time.UTC)
	var reversal ReverseLedgerEntryRecord
	storage := &fakeStorage{
		reverseLedgerEntry: func(_ context.Context, record ReverseLedgerEntryRecord) (domain.LedgerEntry, error) {
			reversal = record
			return domain.LedgerEntry{ID: 3}, nil
		},
		balance: func(_ context.Context, userID domain.UserID) (domain.AmountMinor, error) {
			if userID != 1 {
				t.Fatalf("Balance user ID = %d", userID)
			}
			return 900, nil
		},
		lastLedgerEntries: func(_ context.Context, userID domain.UserID, limit int) ([]domain.LedgerEntry, error) {
			if userID != 1 || limit != 10 {
				t.Fatalf("LastLedgerEntries args = %d, %d", userID, limit)
			}
			return []domain.LedgerEntry{{ID: 2}}, nil
		},
	}
	service := newService(storage, time.Hour, func() time.Time { return now })

	_, err := service.AddAdjustment(context.Background(), AddAdjustmentParams{AmountMinor: 1, AdminTelegramID: 1})
	if !errors.Is(err, ErrAdjustmentNoteRequired) {
		t.Fatalf("AddAdjustment() error = %v", err)
	}
	_, err = service.ReverseLedgerEntry(context.Background(), ReverseLedgerEntryParams{AdminTelegramID: 1})
	if !errors.Is(err, ErrReversalNoteRequired) {
		t.Fatalf("ReverseLedgerEntry() error = %v", err)
	}

	entry, err := service.ReverseLedgerEntry(context.Background(), ReverseLedgerEntryParams{UserID: 1, EntryID: 2, AdminTelegramID: 7, Note: "  mistaken payment  "})
	if err != nil || entry.ID != 3 {
		t.Fatalf("ReverseLedgerEntry() = %#v, %v", entry, err)
	}
	if reversal.UserID != 1 || reversal.EntryID != 2 || reversal.CreatedByTelegramID != 7 || reversal.Note != "mistaken payment" || !reversal.OccurredAt.Equal(now) || !reversal.CreatedAt.Equal(now) {
		t.Fatalf("reversal record = %#v", reversal)
	}
	if balance, err := service.Balance(context.Background(), 1); err != nil || balance != 900 {
		t.Fatalf("Balance() = %d, %v", balance, err)
	}
	if entries, err := service.LastLedgerEntries(context.Background(), 1); err != nil || len(entries) != 1 || entries[0].ID != 2 {
		t.Fatalf("LastLedgerEntries() = %#v, %v", entries, err)
	}
}
