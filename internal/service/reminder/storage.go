package reminder

import (
	"context"
	"time"

	"github.com/Nergous/vpn-balance-bot/internal/domain"
)

type Storage interface {
	CreateReminderDelivery(context.Context, domain.ReminderDelivery) (domain.ReminderDelivery, bool, error)
	UpdateReminderDelivery(context.Context, domain.ReminderDelivery) error
	MarkPendingUnknown(context.Context, time.Time) (int, error)
}
