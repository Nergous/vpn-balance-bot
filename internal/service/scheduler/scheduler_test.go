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
	var calls int
	s.billing = billingFunc(func(context.Context, domain.Date) (int, error) { calls++; <-block; return 0, nil })
	var group sync.WaitGroup
	group.Add(1)
	go func() { defer group.Done(); s.RunOnce(context.Background()) }()
	time.Sleep(time.Millisecond)
	s.RunOnce(context.Background())
	close(block)
	group.Wait()
	if calls != 1 {
		t.Fatalf("calls=%d", calls)
	}
}

type billingFunc func(context.Context, domain.Date) (int, error)

func (f billingFunc) CatchUp(c context.Context, d domain.Date) (int, error) { return f(c, d) }

type reminderFunc func(context.Context, domain.Date) (int, error)

func (f reminderFunc) Process(c context.Context, d domain.Date) (int, error) { return f(c, d) }
