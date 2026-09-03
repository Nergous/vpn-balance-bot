package sqlite

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/Nergous/vpn-balance-bot/internal/domain"
)

func TestProcessTelegramUpdateDeduplicatesAndRollsBackFailures(t *testing.T) {
	ctx := context.Background()
	store := newLedgerStore(t, ctx)
	now := time.Date(2026, time.September, 2, 10, 0, 0, 0, time.UTC)
	user := createLedgerUser(t, store, ctx, 1, now)
	calls := 0
	handler := func(handlerCtx context.Context) error {
		calls++
		_, err := store.CreateLedgerEntry(handlerCtx, newLedgerEntry(user.ID, domain.LedgerKindPayment, 100, now))
		return err
	}
	processed, err := store.ProcessTelegramUpdate(ctx, 42, handler)
	if err != nil || !processed {
		t.Fatalf("first update = %t, %v", processed, err)
	}
	processed, err = store.ProcessTelegramUpdate(ctx, 42, handler)
	if err != nil || processed || calls != 1 {
		t.Fatalf("duplicate update = %t, %v, calls=%d", processed, err, calls)
	}

	sentinel := errors.New("handler failed")
	processed, err = store.ProcessTelegramUpdate(ctx, 43, func(handlerCtx context.Context) error {
		if _, createErr := store.CreateLedgerEntry(handlerCtx, newLedgerEntry(user.ID, domain.LedgerKindPayment, 50, now)); createErr != nil {
			return createErr
		}
		return sentinel
	})
	if processed || !errors.Is(err, sentinel) {
		t.Fatalf("failed update = %t, %v", processed, err)
	}
	processed, err = store.ProcessTelegramUpdate(ctx, 43, func(context.Context) error { return nil })
	if err != nil || !processed {
		t.Fatalf("retried update = %t, %v", processed, err)
	}
	balance, err := store.Balance(ctx, user.ID)
	if err != nil || balance != 100 {
		t.Fatalf("balance = %d, %v, want 100", balance, err)
	}
}
