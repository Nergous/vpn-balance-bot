package account

import (
	"context"
	"time"

	"github.com/Nergous/vpn-balance-bot/internal/domain"
)

// PaymentDraftRecord is a durable, unconfirmed administrator payment.
type PaymentDraftRecord struct {
	AdminTelegramID int64
	UserID          domain.UserID
	AmountMinor     domain.AmountMinor
	Note            *string
	UpdatedAt       time.Time
}

// PaymentDraftStorage is the optional persistence capability used by the
// Telegram payment confirmation flow.
type PaymentDraftStorage interface {
	SavePaymentDraft(context.Context, PaymentDraftRecord) error
	PaymentDraft(context.Context, int64) (PaymentDraftRecord, bool, error)
	DeletePaymentDraft(context.Context, int64) error
}

// CreateUserParams contains inputs for a new customer billing profile.
type CreateUserParams struct {
	AdminTelegramID  int64
	TelegramUserID   *int64
	TelegramChatID   *int64
	Username         *string
	DisplayName      string
	MonthlyFeeMinor  domain.AmountMinor
	Currency         string
	BillingAnchorDay int
	NextChargeOn     domain.Date
}

// UserFilter narrows a user list. A nil Status includes every user.
type UserFilter struct {
	Status *domain.UserStatus
}

// UserStatusCounts contains aggregate profile counts for the admin dashboard.
type UserStatusCounts struct {
	Total        int
	Active       int
	Paused       int
	Disabled     int
	Debtors      int
	Insufficient int
	Unlinked     int
	Unreachable  int
}

// ChangeMonthlyFeeParams contains a profile-only tariff change.
type ChangeMonthlyFeeParams struct {
	AdminTelegramID int64
	UserID          domain.UserID
	MonthlyFeeMinor domain.AmountMinor
}

// ResumeParams requires the next billing date after a paused profile resumes.
type ResumeParams struct {
	AdminTelegramID int64
	UserID          domain.UserID
	NextChargeOn    *domain.Date
}

// AdminUserParams identifies an administrator and one profile mutation target.
type AdminUserParams struct {
	AdminTelegramID int64
	UserID          domain.UserID
}

// CreateInviteTokenParams identifies the administrator creating an invite.
type CreateInviteTokenParams struct {
	AdminTelegramID int64
	UserID          domain.UserID
}

// ConsumeInviteParams contains a raw token presented by a Telegram account.
type ConsumeInviteParams struct {
	Token          string
	TelegramUserID int64
	TelegramChatID int64
}

// CreateInviteTokenRecord is a hash-only persistence request.
type CreateInviteTokenRecord struct {
	TokenHash string
	UserID    domain.UserID
	ExpiresAt time.Time
	CreatedAt time.Time
}

// ConsumeInviteTokenRecord is a hash-only binding request.
type ConsumeInviteTokenRecord struct {
	TokenHash      string
	TelegramUserID int64
	TelegramChatID int64
	ConsumedAt     time.Time
}

// AddOpeningBalanceParams records the signed balance before regular billing begins.
type AddOpeningBalanceParams struct {
	UserID          domain.UserID
	AmountMinor     domain.AmountMinor
	AdminTelegramID int64
	Note            *string
}

// AddPaymentParams records a positive manual payment.
type AddPaymentParams struct {
	UserID          domain.UserID
	AmountMinor     domain.AmountMinor
	AdminTelegramID int64
	Note            *string
	OccurredAt      *time.Time
}

// AddAdjustmentParams records a signed correction with an audit note.
type AddAdjustmentParams struct {
	UserID          domain.UserID
	AmountMinor     domain.AmountMinor
	AdminTelegramID int64
	Note            string
}

// ReverseLedgerEntryParams reverses exactly one entry for the same user.
type ReverseLedgerEntryParams struct {
	UserID          domain.UserID
	EntryID         int64
	AdminTelegramID int64
	Note            string
}

// ReverseLedgerEntryRecord is the atomic persistence request for a reversal.
type ReverseLedgerEntryRecord struct {
	UserID              domain.UserID
	EntryID             int64
	CreatedByTelegramID int64
	Note                string
	OccurredAt          time.Time
	CreatedAt           time.Time
}
