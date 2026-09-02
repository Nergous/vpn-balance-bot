package scheduler

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/Nergous/vpn-balance-bot/internal/domain"
)

func TestRunOnceOrdersBillingBeforeReminders(t *testing.T) {
	order := []string{}
	b := billingFunc(func(context.Context, domain.Date) (int, error) { order = append(order, "billing"); return 2, nil })
	r := reminderFunc(func(context.Context, domain.Date) (int, error) { order = append(order, "reminder"); return 3, nil })
	s, _ := New(b, r, time.UTC, 9)
	s.now = func() time.Time { return time.Date(2026, time.August, 28, 10, 0, 0, 0, time.UTC) }
	result := s.RunOnce(context.Background())
	if result.Charges != 2 || result.Reminders != 3 || len(order) != 2 || order[0] != "billing" {
		t.Fatalf("result=%#v order=%#v", result, order)
	}
}
func TestRunOnceStopsReminderOnBillingFailure(t *testing.T) {
	b := billingFunc(func(context.Context, domain.Date) (int, error) { return 0, errors.New("billing") })
	r := reminderFunc(func(context.Context, domain.Date) (int, error) { t.Fatal("reminder called"); return 0, nil })
	s, _ := New(b, r, time.UTC, 9)
	if s.RunOnce(context.Background()).BillingErr == nil {
		t.Fatal("missing billing error")
	}
}
func TestNextRunAndOverlap(t *testing.T) {
	s, _ := New(billingFunc(func(context.Context, domain.Date) (int, error) { return 0, nil }), reminderFunc(func(context.Context, domain.Date) (int, error) { return 0, nil }), time.UTC, 9)
	next := s.nextRun(time.Date(2026, time.August, 28, 10, 0, 0, 0, time.UTC))
	if next.Hour() != 9 || next.Day() != 29 {
		t.Fatalf("next=%s", next)
	}
	block := make(chan struct{})
	started := make(chan struct{})
	var calls int
	s.billing = billingFunc(func(context.Context, domain.Date) (int, error) {
		calls++
		close(started)
		<-block
		return 0, nil
	})
	var group sync.WaitGroup
	group.Add(1)
	go func() { defer group.Done(); s.RunOnce(context.Background()) }()
	<-started
	if result := s.RunOnce(context.Background()); !result.Skipped {
		t.Fatalf("overlap result = %#v", result)
	}
	close(block)
	group.Wait()
	if calls != 1 {
		t.Fatalf("calls=%d", calls)
	}
}

func TestRunOnceReportsEveryResult(t *testing.T) {
	observed := make(chan Result, 1)
	s, err := New(
		billingFunc(func(context.Context, domain.Date) (int, error) { return 2, nil }),
		reminderFunc(func(context.Context, domain.Date) (int, error) { return 3, nil }),
		time.UTC,
		9,
		WithObserver(func(result Result) { observed <- result }),
	)
	if err != nil {
		t.Fatal(err)
	}
	s.now = func() time.Time { return time.Date(2026, time.September, 2, 10, 0, 0, 0, time.UTC) }

	want := s.RunOnce(context.Background())
	if got := <-observed; got.Date != want.Date || got.Charges != 2 || got.Reminders != 3 {
		t.Fatalf("observed = %#v, want %#v", got, want)
	}
}

func TestRunCurrentDateRetriesRetryableFailureWithBackoff(t *testing.T) {
	var billingCalls, reminderCalls int
	s, _ := New(
		billingFunc(func(context.Context, domain.Date) (int, error) {
			billingCalls++
			if billingCalls == 1 {
				return 1, testRetryableError{}
			}
			return 1, nil
		}),
		reminderFunc(func(context.Context, domain.Date) (int, error) {
			reminderCalls++
			return 1, nil
		}),
		time.UTC,
		9,
	)
	s.now = func() time.Time { return time.Date(2026, time.September, 2, 10, 0, 0, 0, time.UTC) }
	var waits []time.Duration
	s.wait = func(context.Context, time.Duration) bool {
		waits = append(waits, schedulerRetryBackoff(len(waits)+1))
		return true
	}

	s.runCurrentDate(context.Background())
	if billingCalls != 2 || reminderCalls != 1 || len(waits) != 1 || waits[0] != time.Minute {
		t.Fatalf("billing=%d reminders=%d waits=%v", billingCalls, reminderCalls, waits)
	}
}

func TestRunCurrentDateBoundsRetries(t *testing.T) {
	var calls int
	s, _ := New(
		billingFunc(func(context.Context, domain.Date) (int, error) {
			calls++
			return 0, testRetryableError{}
		}),
		reminderFunc(func(context.Context, domain.Date) (int, error) { return 0, nil }),
		time.UTC,
		9,
	)
	s.now = func() time.Time { return time.Date(2026, time.September, 2, 10, 0, 0, 0, time.UTC) }
	s.wait = func(context.Context, time.Duration) bool { return true }

	s.runCurrentDate(context.Background())
	if calls != maxRunAttemptsPerDate {
		t.Fatalf("calls = %d, want %d", calls, maxRunAttemptsPerDate)
	}
}

type testRetryableError struct{}

func (testRetryableError) Error() string { return "retryable" }

func (testRetryableError) Retryable() bool { return true }

type billingFunc func(context.Context, domain.Date) (int, error)

func (f billingFunc) CatchUp(c context.Context, d domain.Date) (int, error) { return f(c, d) }

type reminderFunc func(context.Context, domain.Date) (int, error)

func (f reminderFunc) Process(c context.Context, d domain.Date) (int, error) { return f(c, d) }
