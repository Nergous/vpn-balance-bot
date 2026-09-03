package telegram

import (
	"context"
	"errors"
	"sync"
	"time"

	"github.com/Nergous/vpn-balance-bot/internal/domain"
	"github.com/Nergous/vpn-balance-bot/internal/service/account"
)

// ErrWizardNotFound means that a wizard was cancelled, confirmed, or never started.
var ErrWizardNotFound = errors.New("wizard state not found")

// PaymentDraft stores an unconfirmed payment form.
type PaymentDraft struct {
	UserID      domain.UserID
	AmountMinor domain.AmountMinor
	Note        *string
	OccurredAt  time.Time
}

// PaymentDraftStore persists drafts across restarts.
type PaymentDraftStore interface {
	SavePaymentDraft(context.Context, int64, domain.UserID, domain.AmountMinor, *string, time.Time) (time.Time, error)
	PaymentDraft(context.Context, int64) (domain.UserID, domain.AmountMinor, *string, time.Time, bool, error)
	DeletePaymentDraft(context.Context, int64) error
}

// Wizard owns admin payment drafts, using durable storage in production.
type Wizard struct {
	mu       sync.Mutex
	payments map[int64]PaymentDraft
	store    PaymentDraftStore
}

// NewWizard creates a payment wizard. Tests may omit storage for an in-memory fallback.
func NewWizard(stores ...PaymentDraftStore) *Wizard {
	w := &Wizard{
		payments: make(map[int64]PaymentDraft),
	}
	if len(stores) > 0 {
		w.store = stores[0]
	}
	return w
}

// BeginPayment replaces the admin's previous unconfirmed payment draft.
func (w *Wizard) BeginPayment(ctx context.Context, adminID int64, draft PaymentDraft) (PaymentDraft, error) {
	if w.store != nil {
		occurredAt, err := w.store.SavePaymentDraft(ctx, adminID, draft.UserID, draft.AmountMinor, draft.Note, draft.OccurredAt)
		draft.OccurredAt = occurredAt
		return draft, err
	}
	w.mu.Lock()
	defer w.mu.Unlock()
	w.payments[adminID] = draft
	return draft, nil
}

// Cancel discards the admin's unfinished payment without changing the database.
func (w *Wizard) Cancel(ctx context.Context, adminID int64) error {
	if w.store != nil {
		return w.store.DeletePaymentDraft(ctx, adminID)
	}
	w.mu.Lock()
	defer w.mu.Unlock()
	delete(w.payments, adminID)
	return nil
}

// ConfirmPayment consumes a draft before creating its ledger entry, preventing duplicates.
func (w *Wizard) ConfirmPayment(ctx context.Context, adminID int64, service AdminAccountService) error {
	var draft PaymentDraft
	var ok bool
	if w.store != nil {
		userID, amount, note, occurredAt, found, err := w.store.PaymentDraft(ctx, adminID)
		if err != nil {
			return err
		}
		draft, ok = PaymentDraft{UserID: userID, AmountMinor: amount, Note: note, OccurredAt: occurredAt}, found
	} else {
		w.mu.Lock()
		draft, ok = w.payments[adminID]
		w.mu.Unlock()
	}
	if !ok {
		return ErrWizardNotFound
	}
	params := account.AddPaymentParams{
		UserID:          draft.UserID,
		AmountMinor:     draft.AmountMinor,
		AdminTelegramID: adminID,
		Note:            draft.Note,
	}
	if !draft.OccurredAt.IsZero() {
		params.OccurredAt = &draft.OccurredAt
	}
	_, err := service.AddPayment(
		ctx,
		params,
	)

	if err != nil {
		return err
	}
	return w.Cancel(ctx, adminID)
}
