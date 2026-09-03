package billing

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/Nergous/vpn-balance-bot/internal/domain"
)

func TestNewRejectsNilStorage(t *testing.T) {
	service, err := New(nil)
	if !errors.Is(err, ErrNilStorage) || service != nil {
		t.Fatalf("New(nil) = %#v, %v", service, err)
	}
}

func TestCatchUpRejectsInvalidDate(t *testing.T) {
	storage := &fakeStorage{}
	service := newService(storage, time.Now)
	_, err := service.CatchUp(context.Background(), domain.Date{Year: 2026, Month: time.February, Day: 30})
	if !errors.Is(err, ErrInvalidBillingDate) {
		t.Fatalf("CatchUp() error = %v, want %v", err, ErrInvalidBillingDate)
	}
	if storage.usersDueCalls != 0 {
		t.Fatal("CatchUp() called storage for invalid date")
	}
}

func TestCatchUpRepeatsUntilNoPeriodsDue(t *testing.T) {
	first := testDate(t, 2026, time.January, 31)
	second := testDate(t, 2026, time.February, 28)
	user := domain.User{ID: 1, NextChargeOn: first}
	storage := &fakeStorage{}
	storage.usersDue = func(context.Context, domain.Date, int) ([]domain.User, error) {
		storage.usersDueCalls++
		switch storage.usersDueCalls {
		case 1:
			return []domain.User{user}, nil
		case 2:
			user.NextChargeOn = second
			return []domain.User{user}, nil
		default:
			return nil, nil
		}
	}
	storage.reserve = func(_ context.Context, params ReserveSubscriptionChargeParams) (domain.LedgerEntry, bool, error) {
		if params.UserID != 1 {
			t.Fatalf("Reserve user ID = %d", params.UserID)
		}
		return domain.LedgerEntry{}, true, nil
	}
	service := newService(storage, func() time.Time { return time.Date(2026, time.March, 1, 0, 0, 0, 0, time.UTC) })

	created, err := service.CatchUp(context.Background(), testDate(t, 2026, time.March, 1))
	if err != nil || created != 2 {
		t.Fatalf("CatchUp() = %d, %v, want 2", created, err)
	}
}

func TestCatchUpProcessesBacklogAcrossBatches(t *testing.T) {
	periods := []domain.Date{
		testDate(t, 2026, time.January, 31),
		testDate(t, 2026, time.February, 28),
		testDate(t, 2026, time.March, 31),
	}
	next := 0
	storage := &fakeStorage{
		usersDue: func(_ context.Context, _ domain.Date, limit int) ([]domain.User, error) {
			if limit < 1 {
				t.Fatalf("limit = %d", limit)
			}
			if next == len(periods) {
				return nil, nil
			}
			return []domain.User{{ID: 1, NextChargeOn: periods[next]}}, nil
		},
		reserve: func(_ context.Context, params ReserveSubscriptionChargeParams) (domain.LedgerEntry, bool, error) {
			if params.BillingPeriodOn != periods[next] {
				t.Fatalf("billing period = %s, want %s", params.BillingPeriodOn, periods[next])
			}
			next++
			return domain.LedgerEntry{}, true, nil
		},
	}
	service := newService(storage, time.Now)
	service.chargeBatchSize = 2

	created, err := service.CatchUp(context.Background(), periods[len(periods)-1])
	if created != 3 || err != nil {
		t.Fatalf("CatchUp() = %d, %v", created, err)
	}

	created, err = service.CatchUp(context.Background(), periods[len(periods)-1])
	if created != 0 || err != nil {
		t.Fatalf("idempotent CatchUp() = %d, %v", created, err)
	}
}

type fakeStorage struct {
	usersDue      func(context.Context, domain.Date, int) ([]domain.User, error)
	reserve       func(context.Context, ReserveSubscriptionChargeParams) (domain.LedgerEntry, bool, error)
	usersDueCalls int
}

func (f *fakeStorage) UsersDueForCharge(ctx context.Context, asOf domain.Date, limit int) ([]domain.User, error) {
	if f.usersDue == nil {
		return nil, errors.New("unexpected UsersDueForCharge call")
	}
	return f.usersDue(ctx, asOf, limit)
}

func (f *fakeStorage) ReserveSubscriptionCharge(ctx context.Context, params ReserveSubscriptionChargeParams) (domain.LedgerEntry, bool, error) {
	if f.reserve == nil {
		return domain.LedgerEntry{}, false, errors.New("unexpected ReserveSubscriptionCharge call")
	}
	return f.reserve(ctx, params)
}

func testDate(t *testing.T, year int, month time.Month, day int) domain.Date {
	t.Helper()
	date, err := domain.NewDate(year, month, day)
	if err != nil {
		t.Fatal(err)
	}
	return date
}
