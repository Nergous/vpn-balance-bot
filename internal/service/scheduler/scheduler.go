package scheduler

import (
	"context"
	"errors"
	"sync"
	"time"

	"github.com/Nergous/vpn-balance-bot/internal/domain"
)

var ErrNilBilling = errors.New("scheduler billing service is nil")
var ErrNilReminder = errors.New("scheduler reminder service is nil")

type Billing interface {
	CatchUp(context.Context, domain.Date) (int, error)
}
type Reminders interface {
	Process(context.Context, domain.Date) (int, error)
}
type Result struct {
	Date                    domain.Date
	Charges, Reminders      int
	BillingErr, ReminderErr error
}
type Scheduler struct {
	billing   Billing
	reminders Reminders
	location  *time.Location
	hour      int
	now       func() time.Time
	mu        sync.Mutex
}

func New(billing Billing, reminders Reminders, location *time.Location, hour int) (*Scheduler, error) {
	if billing == nil {
		return nil, ErrNilBilling
	}
	if reminders == nil {
		return nil, ErrNilReminder
	}
	if location == nil {
		return nil, domain.ErrNilLocation
	}
	if hour < 0 || hour > 23 {
		return nil, errors.New("scheduler hour is invalid")
	}
	return &Scheduler{billing: billing, reminders: reminders, location: location, hour: hour, now: time.Now}, nil
}
func (s *Scheduler) RunOnce(ctx context.Context) Result {
	if !s.mu.TryLock() {
		return Result{}
	}
	defer s.mu.Unlock()
	date, _ := domain.DateFromTime(s.now(), s.location)
	result := Result{Date: date}
	result.Charges, result.BillingErr = s.billing.CatchUp(ctx, date)
	if result.BillingErr != nil {
		return result
	}
	result.Reminders, result.ReminderErr = s.reminders.Process(ctx, date)
	return result
}
func (s *Scheduler) Start(ctx context.Context) {
	s.RunOnce(ctx)
	for {
		timer := time.NewTimer(time.Until(s.nextRun(s.now())))
		select {
		case <-ctx.Done():
			timer.Stop()
			return
		case <-timer.C:
			s.RunOnce(ctx)
		}
	}
}
func (s *Scheduler) nextRun(now time.Time) time.Time {
	local := now.In(s.location)
	next := time.Date(local.Year(), local.Month(), local.Day(), s.hour, 0, 0, 0, s.location)
	if !next.After(local) {
		next = next.AddDate(0, 0, 1)
	}
	return next
}
