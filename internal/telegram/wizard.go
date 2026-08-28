package telegram

import (
	"context"
	"errors"
	"sync"

	"github.com/Nergous/vpn-balance-bot/internal/domain"
	"github.com/Nergous/vpn-balance-bot/internal/service/account"
)

var ErrWizardNotFound = errors.New("wizard state not found")

type PaymentDraft struct {
	UserID      domain.UserID
	AmountMinor domain.AmountMinor
	Note        *string
}
type Wizard struct {
	mu       sync.Mutex
	payments map[int64]PaymentDraft
}

func NewWizard() *Wizard { return &Wizard{payments: make(map[int64]PaymentDraft)} }
func (w *Wizard) BeginPayment(adminID int64, draft PaymentDraft) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.payments[adminID] = draft
}
func (w *Wizard) Cancel(adminID int64) { w.mu.Lock(); defer w.mu.Unlock(); delete(w.payments, adminID) }
func (w *Wizard) ConfirmPayment(ctx context.Context, adminID int64, service AdminAccountService) error {
	w.mu.Lock()
	draft, ok := w.payments[adminID]
	if ok {
		delete(w.payments, adminID)
	}
	w.mu.Unlock()
	if !ok {
		return ErrWizardNotFound
	}
	_, err := service.AddPayment(ctx, account.AddPaymentParams{UserID: draft.UserID, AmountMinor: draft.AmountMinor, AdminTelegramID: adminID, Note: draft.Note})
	return err
}
