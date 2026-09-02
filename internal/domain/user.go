package domain

import "time"

// UserID identifies a user inside the application database.
type UserID int64

type UserStatus string

const (
	UserStatusActive   UserStatus = "active"
	UserStatusPaused   UserStatus = "paused"
	UserStatusDisabled UserStatus = "disabled"
)

// IsValid reports whether the status is supported by the MVP lifecycle.
func (s UserStatus) IsValid() bool {
	switch s {
	case UserStatusActive, UserStatusPaused, UserStatusDisabled:
		return true
	default:
		return false
	}
}

// CanTransitionTo reports whether the lifecycle permits moving to next.
func (s UserStatus) CanTransitionTo(next UserStatus) bool {
	switch s {
	case UserStatusActive:
		return next == UserStatusPaused || next == UserStatusDisabled
	case UserStatusPaused:
		return next == UserStatusActive || next == UserStatusDisabled
	default:
		return false
	}
}

// User is the billing profile of one VPN customer.
type User struct {
	ID               UserID
	TelegramUserID   *int64
	TelegramChatID   *int64
	Username         *string
	DisplayName      string
	MonthlyFeeMinor  AmountMinor
	Currency         string
	BillingAnchorDay int
	NextChargeOn     Date
	Status           UserStatus
	CreatedAt        time.Time
	UpdatedAt        time.Time
}
