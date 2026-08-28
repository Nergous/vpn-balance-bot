package account

import "errors"

var (
	ErrNotFound            = errors.New("account not found")
	ErrTelegramUserIDTaken = errors.New("Telegram user ID is already assigned")
	ErrTelegramChatIDTaken = errors.New("Telegram chat ID is already assigned")
	ErrNilStorage          = errors.New("account storage is nil")
	ErrInvalidDisplayName  = errors.New("display name is invalid")
	ErrInvalidMonthlyFee   = errors.New("monthly fee must be positive")
	ErrUnsupportedCurrency = errors.New("currency is not supported")
	ErrInvalidAnchorDay    = errors.New("billing anchor day is invalid")
	ErrInvalidNextChargeOn = errors.New("next charge date is invalid")
	ErrInvalidUserStatus   = errors.New("user status is invalid")
	ErrResumeDateRequired  = errors.New("resume requires a next charge date")
	ErrInvalidInviteTTL    = errors.New("invite TTL must be positive")
	ErrInvalidInviteToken  = errors.New("invite token is invalid")
	ErrInviteNotFound      = errors.New("invite token not found")
	ErrInviteExpired       = errors.New("invite token expired")
	ErrInviteAlreadyUsed   = errors.New("invite token already used")
	ErrInviteUserLinked    = errors.New("invite profile is already linked")
	ErrInviteTokenGenerate = errors.New("failed to generate invite token")
)
