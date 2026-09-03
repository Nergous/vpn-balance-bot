package domain

import "time"

type ReminderStatus string

const (
	ReminderStatusPending ReminderStatus = "pending"
	ReminderStatusSent    ReminderStatus = "sent"
	ReminderStatusFailed  ReminderStatus = "failed"
	ReminderStatusSkipped ReminderStatus = "skipped"
)

// IsValid reports whether the delivery status is supported by the MVP.
func (s ReminderStatus) IsValid() bool {
	switch s {
	case ReminderStatusPending,
		ReminderStatusSent,
		ReminderStatusFailed,
		ReminderStatusSkipped:
		return true
	default:
		return false
	}
}

type ReminderType string

const (
	ReminderTypeBeforeCharge ReminderType = "before_charge"
	ReminderTypeChargeDebt   ReminderType = "charge_debt"
	ReminderTypeOverdue3D    ReminderType = "overdue_3d"
	ReminderTypeOverdue7D    ReminderType = "overdue_7d"
	ReminderTypeManual       ReminderType = "manual"
)

// IsValid reports whether the reminder type is supported by the MVP.
func (t ReminderType) IsValid() bool {
	switch t {
	case ReminderTypeBeforeCharge,
		ReminderTypeChargeDebt,
		ReminderTypeOverdue3D,
		ReminderTypeOverdue7D,
		ReminderTypeManual:
		return true
	default:
		return false
	}
}

// ReminderDelivery records one attempted reminder for a billing period.
type ReminderDelivery struct {
	UserID            UserID
	BillingDate       Date
	ReminderType      ReminderType
	DeliveryKey       string
	ScheduledDate     Date
	Status            ReminderStatus
	SentAt            *time.Time
	TelegramMessageID *int64
	ErrorCode         *string
	CreatedAt         time.Time
	UpdatedAt         time.Time
	AttemptCount      int
	LeaseExpiresAt    *time.Time
}
