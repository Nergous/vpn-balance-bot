package scheduler

import (
	"context"
	"errors"
	"sync"
	"time"

	"github.com/Nergous/vpn-balance-bot/internal/domain"
)

// ErrNilBilling indicates that a scheduler was created without a billing service.
var ErrNilBilling = errors.New("scheduler billing service is nil")

// ErrNilReminder indicates that a scheduler was created without a reminder service.
var ErrNilReminder = errors.New("scheduler reminder service is nil")

// ErrNilObserver indicates that WithObserver received a nil callback.
var ErrNilObserver = errors.New("scheduler observer is nil")

const maxRunAttemptsPerDate = 3

// Billing processes subscription charges up to a given date.
type Billing interface {
	CatchUp(context.Context, domain.Date) (int, error)
}

// Reminders processes scheduled reminders for a given date.
type Reminders interface {
	Process(context.Context, domain.Date) (int, error)
}

// Result summarizes one scheduler run.
type Result struct {
	Date                    domain.Date
	Charges, Reminders      int
	BillingErr, ReminderErr error
	Skipped                 bool
}

// NeedsRetry reports whether a service marked any failure safe to retry.
func (r Result) NeedsRetry() bool {
	return isRetryable(r.BillingErr) || isRetryable(r.ReminderErr)
}

// Observer receives every Result returned by RunOnce.
type Observer func(Result)

// Option configures scheduler behavior without breaking existing constructors.
type Option func(*Scheduler) error

// WithObserver injects result reporting. App integration should pass this option.
func WithObserver(observer Observer) Option {
	return func(s *Scheduler) error {
		if observer == nil {
			return ErrNilObserver
		}
		s.observer = observer
		return nil
	}
}

// Scheduler coordinates daily billing and reminder processing.
type Scheduler struct {
	billing   Billing
	reminders Reminders
	location  *time.Location
	hour      int
	now       func() time.Time
	observer  Observer
	wait      func(context.Context, time.Duration) bool
	mu        sync.Mutex
}

// New creates a scheduler that runs daily at hour in location.
func New(billing Billing, reminders Reminders, location *time.Location, hour int, options ...Option) (*Scheduler, error) {
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

	scheduler := &Scheduler{
		billing:   billing,
		reminders: reminders,
		location:  location,
		hour:      hour,
		now:       time.Now,
		observer:  func(Result) {},
		wait:      waitForRetry,
	}
	for _, option := range options {
		if option == nil {
			continue
		}
		if err := option(scheduler); err != nil {
			return nil, err
		}
	}

	return scheduler, nil
}

// RunOnce processes billing and reminders for the current local date.
// Concurrent calls are reported as skipped without starting another run.
func (s *Scheduler) RunOnce(ctx context.Context) (result Result) {
	if !s.mu.TryLock() {
		result.Skipped = true
		s.observer(result)
		return result
	}

	defer func() {
		s.mu.Unlock()
		s.observer(result)
	}()

	date, _ := domain.DateFromTime(s.now(), s.location)
	result.Date = date

	result.Charges, result.BillingErr = s.billing.CatchUp(ctx, date)
	if result.BillingErr != nil {
		return result
	}

	result.Reminders, result.ReminderErr = s.reminders.Process(ctx, date)
	return result
}

// Start runs immediately, then once per day until ctx is canceled.
func (s *Scheduler) Start(ctx context.Context) {
	s.runCurrentDate(ctx)
	for {
		timer := time.NewTimer(time.Until(s.nextRun(s.now())))
		select {
		case <-ctx.Done():
			timer.Stop()
			return
		case <-timer.C:
			s.runCurrentDate(ctx)
		}
	}
}

func (s *Scheduler) runCurrentDate(ctx context.Context) {
	runDate, _ := domain.DateFromTime(s.now(), s.location)
	for attempt := 1; attempt <= maxRunAttemptsPerDate; attempt++ {
		result := s.RunOnce(ctx)
		if result.Skipped || result.Date != runDate || !result.NeedsRetry() || attempt == maxRunAttemptsPerDate {
			return
		}

		if !s.wait(ctx, schedulerRetryBackoff(attempt)) {
			return
		}

		currentDate, _ := domain.DateFromTime(s.now(), s.location)
		if currentDate != runDate {
			return
		}
	}
}

func schedulerRetryBackoff(attempt int) time.Duration {
	return min(time.Minute<<max(attempt-1, 0), 15*time.Minute)
}

func waitForRetry(ctx context.Context, delay time.Duration) bool {
	timer := time.NewTimer(delay)
	defer timer.Stop()

	select {
	case <-ctx.Done():
		return false
	case <-timer.C:
		return true
	}
}

type retryableError interface {
	Retryable() bool
}

func isRetryable(err error) bool {
	var retryable retryableError
	return errors.As(err, &retryable) && retryable.Retryable()
}

func (s *Scheduler) nextRun(now time.Time) time.Time {
	local := now.In(s.location)

	next := time.Date(local.Year(), local.Month(), local.Day(), s.hour, 0, 0, 0, s.location)
	if !next.After(local) {
		next = next.AddDate(0, 0, 1)
	}

	return next
}
