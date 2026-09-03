package sqlite

import (
	"context"
	"testing"
	"time"

	"github.com/Nergous/vpn-balance-bot/internal/service/account"
)

func TestPaymentDraftPersistsAndDeletes(t *testing.T) {
	ctx := context.Background()
	store := newLedgerStore(t, ctx)
	now := time.Date(2026, time.September, 2, 10, 0, 0, 0, time.UTC)
	user := createLedgerUser(t, store, ctx, 1, now)
	note := "cash"
	draft := account.PaymentDraftRecord{AdminTelegramID: 7, UserID: user.ID, AmountMinor: 500, Note: &note, UpdatedAt: now}
	if err := store.SavePaymentDraft(ctx, draft); err != nil {
		t.Fatal(err)
	}
	loaded, found, err := store.PaymentDraft(ctx, 7)
	if err != nil || !found || loaded.UserID != user.ID || loaded.AmountMinor != 500 || loaded.Note == nil || *loaded.Note != note {
		t.Fatalf("PaymentDraft() = %#v, %t, %v", loaded, found, err)
	}
	if err := store.DeletePaymentDraft(ctx, 7); err != nil {
		t.Fatal(err)
	}
	_, found, err = store.PaymentDraft(ctx, 7)
	if err != nil || found {
		t.Fatalf("deleted draft found=%t err=%v", found, err)
	}
}
