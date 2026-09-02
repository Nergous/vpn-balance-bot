package billing

import (
	"context"
	"time"

	"github.com/Nergous/vpn-balance-bot/internal/domain"
)

// Storage defines the transactional persistence operations needed by billing.
type Storage interface {
	UsersDueForCharge(ctx context.Context, asOf domain.Date, limit int) ([]domain.User, error)
	ReserveSubscriptionCharge(ctx context.Context, params ReserveSubscriptionChargeParams) (domain.LedgerEntry, bool, error)
}

// ReserveSubscriptionChargeParams identifies one billing period to process.
type ReserveSubscriptionChargeParams struct {
	UserID          domain.UserID
	BillingPeriodOn domain.Date
	OccurredAt      time.Time
	UpdatedAt       time.Time
}
