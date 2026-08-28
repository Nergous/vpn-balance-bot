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
)
