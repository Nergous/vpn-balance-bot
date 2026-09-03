package reminder

import (
	"context"
	"time"

	"github.com/Nergous/vpn-balance-bot/internal/domain"
)

// Candidate contains all persisted facts needed to select one automatic reminder.
type Candidate struct {
	User                 domain.User
	Balance              domain.AmountMinor
	LatestChargePeriodOn *domain.Date
}

// Storage defines persistence operations required by reminder delivery.
type Storage interface {
	ReminderCandidates(context.Context, domain.Date) ([]Candidate, error)
	ReserveReminderDelivery(context.Context, domain.ReminderDelivery, int) (domain.ReminderDelivery, int, bool, error)
	UpdateReminderDeliveryAttempt(context.Context, domain.ReminderDelivery, int, *time.Time) (bool, error)
	MarkAllPendingUnknown(context.Context, time.Time) (int, error)
	MarkPendingUnknown(context.Context, time.Time) (int, error)
	MarkUnknownRetryable(context.Context, domain.UserID, domain.Date, domain.ReminderType, time.Time, int) (bool, error)
}
