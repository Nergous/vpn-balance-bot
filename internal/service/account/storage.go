package account

import (
	"context"
	"time"

	"github.com/Nergous/vpn-balance-bot/internal/domain"
)

// Storage defines persistence operations required by the account service.
type Storage interface {
	CreateUser(ctx context.Context, user domain.User) (domain.User, error)
	UserByID(ctx context.Context, userID domain.UserID) (domain.User, error)
	ListUsers(ctx context.Context, status *domain.UserStatus) ([]domain.User, error)
	UserByTelegramID(ctx context.Context, telegramID int64) (domain.User, error)
	SetMonthlyFee(ctx context.Context, userID domain.UserID, fee domain.AmountMinor, updatedAt time.Time) (domain.User, error)
	PauseUser(ctx context.Context, userID domain.UserID, updatedAt time.Time) (domain.User, error)
	ResumeUser(ctx context.Context, userID domain.UserID, nextChargeOn domain.Date, updatedAt time.Time) (domain.User, error)
	DisableUser(ctx context.Context, userID domain.UserID, updatedAt time.Time) (domain.User, error)
	CreateInviteToken(ctx context.Context, record CreateInviteTokenRecord) error
	ConsumeInviteToken(ctx context.Context, record ConsumeInviteTokenRecord) (domain.User, error)
	CreateLedgerEntry(ctx context.Context, entry domain.LedgerEntry) (domain.LedgerEntry, error)
	Balance(ctx context.Context, userID domain.UserID) (domain.AmountMinor, error)
	LastLedgerEntries(ctx context.Context, userID domain.UserID, limit int) ([]domain.LedgerEntry, error)
	LastUnreversedPayment(ctx context.Context, userID domain.UserID) (domain.LedgerEntry, bool, error)
	ReverseLedgerEntry(ctx context.Context, record ReverseLedgerEntryRecord) (domain.LedgerEntry, error)
}
