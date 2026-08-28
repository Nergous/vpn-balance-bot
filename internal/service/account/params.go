package account

import "github.com/Nergous/vpn-balance-bot/internal/domain"

// CreateUserParams contains inputs for a new customer billing profile.
type CreateUserParams struct {
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

// ChangeMonthlyFeeParams contains a profile-only tariff change.
type ChangeMonthlyFeeParams struct {
	UserID          domain.UserID
	MonthlyFeeMinor domain.AmountMinor
}

// ResumeParams requires the next billing date after a paused profile resumes.
type ResumeParams struct {
	UserID       domain.UserID
	NextChargeOn *domain.Date
}
