package billing

import (
	"context"
	"time"

	"github.com/Nergous/vpn-balance-bot/internal/domain"
)

// Service performs idempotent subscription billing catch-up.
type Service struct {
	storage Storage
	now     func() time.Time
}

func New(storage Storage) (*Service, error) {
	if storage == nil {
		return nil, ErrNilStorage
	}
	return newService(storage, time.Now), nil
}

func newService(storage Storage, now func() time.Time) *Service {
	return &Service{storage: storage, now: now}
}

// CatchUp charges every active user for all periods due on or before asOf.
// It returns the number of newly created subscription charges.
func (s *Service) CatchUp(ctx context.Context, asOf domain.Date) (int, error) {
	if !asOf.IsValid() {
		return 0, ErrInvalidBillingDate
	}

	chargesCreated := 0
	for {
		users, err := s.storage.UsersDueForCharge(ctx, asOf)
		if err != nil {
			return chargesCreated, err
		}
		if len(users) == 0 {
			return chargesCreated, nil
		}

		madeProgress := false
		for _, user := range users {
			now := s.now().UTC().Truncate(time.Second)
			_, created, err := s.storage.ReserveSubscriptionCharge(ctx, ReserveSubscriptionChargeParams{
				UserID: user.ID, BillingPeriodOn: user.NextChargeOn,
				OccurredAt: now, UpdatedAt: now,
			})
			if err != nil {
				return chargesCreated, err
			}
			if created {
				chargesCreated++
				madeProgress = true
			}
		}

		if !madeProgress {
			return chargesCreated, nil
		}
	}
}
