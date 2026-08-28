package account

import (
	"context"
	"strings"

	"github.com/Nergous/vpn-balance-bot/internal/domain"
)

const lastLedgerEntriesLimit = 10

// AddOpeningBalance records a signed opening balance.
func (s *Service) AddOpeningBalance(ctx context.Context, params AddOpeningBalanceParams) (domain.LedgerEntry, error) {
	if err := validateManualLedgerParams(params.AmountMinor, params.AdminTelegramID); err != nil {
		return domain.LedgerEntry{}, err
	}

	return s.createManualLedgerEntry(ctx, params.UserID, domain.LedgerKindOpeningBalance, params.AmountMinor, params.AdminTelegramID, params.Note)
}

// AddPayment records a positive payment.
func (s *Service) AddPayment(ctx context.Context, params AddPaymentParams) (domain.LedgerEntry, error) {
	if params.AmountMinor <= 0 {
		return domain.LedgerEntry{}, ErrInvalidPaymentAmount
	}
	if params.AdminTelegramID <= 0 {
		return domain.LedgerEntry{}, ErrInvalidAdminTelegramID
	}

	return s.createManualLedgerEntry(ctx, params.UserID, domain.LedgerKindPayment, params.AmountMinor, params.AdminTelegramID, params.Note)
}

// AddAdjustment records a signed correction with a mandatory note.
func (s *Service) AddAdjustment(ctx context.Context, params AddAdjustmentParams) (domain.LedgerEntry, error) {
	if err := validateManualLedgerParams(params.AmountMinor, params.AdminTelegramID); err != nil {
		return domain.LedgerEntry{}, err
	}
	note := strings.TrimSpace(params.Note)
	if note == "" {
		return domain.LedgerEntry{}, ErrAdjustmentNoteRequired
	}

	return s.createManualLedgerEntry(ctx, params.UserID, domain.LedgerKindAdjustment, params.AmountMinor, params.AdminTelegramID, &note)
}

// ReverseLedgerEntry creates an opposite-signed immutable reversal.
func (s *Service) ReverseLedgerEntry(ctx context.Context, params ReverseLedgerEntryParams) (domain.LedgerEntry, error) {
	if params.AdminTelegramID <= 0 {
		return domain.LedgerEntry{}, ErrInvalidAdminTelegramID
	}
	note := strings.TrimSpace(params.Note)
	if note == "" {
		return domain.LedgerEntry{}, ErrReversalNoteRequired
	}

	now := s.nowUTC()
	return s.storage.ReverseLedgerEntry(ctx, ReverseLedgerEntryRecord{
		UserID: params.UserID, EntryID: params.EntryID,
		CreatedByTelegramID: params.AdminTelegramID, Note: note,
		OccurredAt: now, CreatedAt: now,
	})
}

// Balance calculates the current signed ledger sum for a user.
func (s *Service) Balance(ctx context.Context, userID domain.UserID) (domain.AmountMinor, error) {
	return s.storage.Balance(ctx, userID)
}

// LastLedgerEntries returns up to ten newest immutable ledger entries.
func (s *Service) LastLedgerEntries(ctx context.Context, userID domain.UserID) ([]domain.LedgerEntry, error) {
	return s.storage.LastLedgerEntries(ctx, userID, lastLedgerEntriesLimit)
}

func (s *Service) createManualLedgerEntry(ctx context.Context, userID domain.UserID, kind domain.LedgerKind, amount domain.AmountMinor, adminTelegramID int64, note *string) (domain.LedgerEntry, error) {
	now := s.nowUTC()
	adminID := adminTelegramID
	return s.storage.CreateLedgerEntry(ctx, domain.LedgerEntry{
		UserID: userID, Kind: kind, AmountMinor: amount,
		OccurredAt: now, CreatedByTelegramID: &adminID, Note: note, CreatedAt: now,
	})
}

func validateManualLedgerParams(amount domain.AmountMinor, adminTelegramID int64) error {
	if amount == 0 {
		return ErrInvalidLedgerAmount
	}
	if adminTelegramID <= 0 {
		return ErrInvalidAdminTelegramID
	}
	return nil
}
