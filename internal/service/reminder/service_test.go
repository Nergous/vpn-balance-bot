package reminder

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/Nergous/vpn-balance-bot/internal/domain"
	"github.com/Nergous/vpn-balance-bot/internal/localization"
)

func TestDeliverReservesBeforeSending(t *testing.T) {
	chatID := int64(10)
	now := time.Date(2026, time.August, 28, 0, 0, 0, 0, time.UTC)
	storage := &fakeStorage{reserved: true, attempt: 1}
	sender := &fakeSender{messageID: 7}
	service, err := New(storage, sender, localization.Russian)
	if err != nil {
		t.Fatal(err)
	}
	service.now = func() time.Time { return now }
	date, _ := domain.NewDate(2026, time.September, 1)
	created, err := service.Deliver(context.Background(), domain.User{ID: 1, TelegramChatID: &chatID}, date, date, domain.ReminderTypeManual, "test")
	if err != nil || !created || sender.calls != 1 || len(storage.updated) != 1 || storage.updated[0].Status != domain.ReminderStatusSent {
		t.Fatalf("Deliver() = %t, %v, sender=%d, updates=%#v", created, err, sender.calls, storage.updated)
	}
}

func TestDeliverDoesNotSendDuplicateOrFailure(t *testing.T) {
	chatID := int64(10)
	date, _ := domain.NewDate(2026, time.September, 1)
	storage := &fakeStorage{reserved: false, attempt: 1}
	sender := &fakeSender{}
	service, _ := New(storage, sender, localization.Russian)
	created, err := service.Deliver(context.Background(), domain.User{ID: 1, TelegramChatID: &chatID}, date, date, domain.ReminderTypeManual, "test")
	if err != nil || created || sender.calls != 0 {
		t.Fatalf("duplicate = %t, %v, calls=%d", created, err, sender.calls)
	}

	storage.reserved = true
	senderErr := errors.New("network")
	sender.err = senderErr
	created, err = service.Deliver(context.Background(), domain.User{ID: 1, TelegramChatID: &chatID}, date, date, domain.ReminderTypeManual, "test")
	if !errors.Is(err, senderErr) || created || len(storage.updated) != 1 || storage.updated[0].Status != domain.ReminderStatusFailed {
		t.Fatalf("failure = %t, %v, updates=%#v", created, err, storage.updated)
	}
}

func TestDeliverJoinsSenderAndPersistenceErrors(t *testing.T) {
	chatID := int64(10)
	date := mustDate(t, 2026, time.September, 1)
	senderErr := errors.New("transport")
	persistenceErr := errors.New("persistence")
	storage := &fakeStorage{reserved: true, attempt: 1, updateErr: persistenceErr}
	service, _ := New(storage, &fakeSender{err: senderErr}, localization.Russian)

	created, err := service.Deliver(context.Background(), domain.User{ID: 1, TelegramChatID: &chatID}, date, date, domain.ReminderTypeManual, "test")
	if created || !errors.Is(err, senderErr) || !errors.Is(err, persistenceErr) {
		t.Fatalf("Deliver() = %t, %v", created, err)
	}
}

func TestDeliverOnlyMarksDefiniteFailuresRetryable(t *testing.T) {
	chatID := int64(10)
	date := mustDate(t, 2026, time.September, 1)
	for _, tc := range []struct {
		name          string
		code          DeliveryErrorCode
		attempt       int
		wantRetry     bool
		wantRetryWait time.Duration
	}{
		{"definite transient", DeliveryErrorRetryable, 1, true, time.Minute},
		{"ambiguous", DeliveryErrorUnknown, 1, false, 0},
		{"sent outcome unavailable", DeliveryErrorFailed, 1, false, 0},
		{"retry limit", DeliveryErrorRetryable, maxDeliveryAttempts, false, 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			senderErr := errors.New("send")
			storage := &fakeStorage{reserved: true, attempt: tc.attempt}
			service, _ := New(storage, &fakeSender{err: senderErr, code: tc.code}, localization.Russian)
			service.now = func() time.Time { return time.Date(2026, time.September, 1, 12, 0, 0, 0, time.UTC) }

			_, err := service.Deliver(context.Background(), domain.User{ID: 1, TelegramChatID: &chatID}, date, date, domain.ReminderTypeManual, "test")
			if !errors.Is(err, senderErr) || errors.Is(err, ErrRetryableDelivery) != tc.wantRetry {
				t.Fatalf("error = %v, retryable=%t", err, errors.Is(err, ErrRetryableDelivery))
			}
			if tc.wantRetry {
				if storage.retryAt[0] == nil || storage.retryAt[0].Sub(service.now()) != tc.wantRetryWait {
					t.Fatalf("retry at = %#v", storage.retryAt[0])
				}
			} else if storage.retryAt[0] != nil {
				t.Fatalf("unexpected retry at %s", storage.retryAt[0])
			}
		})
	}
}

func TestProcessSelectsAndDeliversAutomaticReminder(t *testing.T) {
	chatID := int64(10)
	next := mustDate(t, 2026, time.September, 10)
	today := mustDate(t, 2026, time.September, 7)
	storage := &fakeStorage{
		reserved: true,
		attempt:  1,
		candidates: []Candidate{{
			User: domain.User{
				ID: 1, TelegramChatID: &chatID, Status: domain.UserStatusActive,
				MonthlyFeeMinor: 100, NextChargeOn: next,
			},
			Balance: 0,
		}},
	}
	sender := &fakeSender{messageID: 7}
	service, err := New(storage, sender, localization.Russian)
	if err != nil {
		t.Fatal(err)
	}
	service.now = func() time.Time { return time.Date(2026, time.September, 7, 0, 0, 0, 0, time.UTC) }

	delivered, err := service.Process(context.Background(), today)
	if err != nil {
		t.Fatal(err)
	}
	if delivered != 1 || sender.calls != 1 || len(storage.updated) != 1 {
		t.Fatalf("Process() = %d, sender=%d, updates=%#v", delivered, sender.calls, storage.updated)
	}
	if storage.updated[0].ReminderType != domain.ReminderTypeBeforeCharge {
		t.Fatalf("type = %q", storage.updated[0].ReminderType)
	}
}

func TestDeliverClassifiesAmbiguousAndUnreachableErrors(t *testing.T) {
	chatID := int64(10)
	date := mustDate(t, 2026, time.September, 1)
	for _, tc := range []struct {
		name string
		err  error
		code DeliveryErrorCode
	}{
		{"ambiguous timeout", context.DeadlineExceeded, DeliveryErrorUnknown},
		{"unreachable", errors.New("forbidden"), DeliveryErrorOffline},
	} {
		t.Run(tc.name, func(t *testing.T) {
			storage := &fakeStorage{reserved: true, attempt: 1}
			sender := &fakeSender{err: tc.err, code: tc.code}
			service, _ := New(storage, sender, localization.Russian)
			_, err := service.Deliver(context.Background(), domain.User{ID: 1, TelegramChatID: &chatID}, date, date, domain.ReminderTypeManual, "test")
			if !errors.Is(err, tc.err) {
				t.Fatalf("Deliver() error = %v", err)
			}
			if got := *storage.updated[0].ErrorCode; got != string(tc.code) {
				t.Fatalf("error code = %q, want %q", got, tc.code)
			}
		})
	}
}

func TestProcessContinuesAfterUserError(t *testing.T) {
	chatID := int64(10)
	next := mustDate(t, 2026, time.September, 10)
	today := mustDate(t, 2026, time.September, 7)
	storage := &fakeStorage{
		reserved: true,
		attempt:  1,
		candidates: []Candidate{
			{User: domain.User{ID: 1, TelegramChatID: &chatID, Status: domain.UserStatusActive, MonthlyFeeMinor: 100, NextChargeOn: next}, Balance: 0},
			{User: domain.User{ID: 2, TelegramChatID: &chatID, Status: domain.UserStatusActive, MonthlyFeeMinor: 100, NextChargeOn: next}, Balance: 0},
		},
	}
	service, _ := New(storage, &fakeSender{errs: []error{errors.New("transport"), nil}}, localization.Russian)
	delivered, err := service.Process(context.Background(), today)
	if delivered != 1 || err == nil {
		t.Fatalf("Process() = %d, %v", delivered, err)
	}
}

func TestProcessUsesLatestChargedPeriodAfterCatchUp(t *testing.T) {
	chatID := int64(10)
	charged := mustDate(t, 2026, time.September, 1)
	next := mustDate(t, 2026, time.October, 1)
	storage := &fakeStorage{
		reserved: true,
		attempt:  1,
		candidates: []Candidate{{
			User: domain.User{
				ID: 1, TelegramChatID: &chatID, Status: domain.UserStatusActive,
				MonthlyFeeMinor: 100, NextChargeOn: next,
			},
			Balance:              -100,
			LatestChargePeriodOn: &charged,
		}},
	}
	service, _ := New(storage, &fakeSender{messageID: 7}, localization.Russian)

	delivered, err := service.Process(context.Background(), charged)
	if err != nil || delivered != 1 {
		t.Fatalf("Process() = %d, %v", delivered, err)
	}
	if got := storage.reservations[0]; got.BillingDate != charged || got.ReminderType != domain.ReminderTypeChargeDebt {
		t.Fatalf("reservation = %#v", got)
	}
}

func TestProcessRetriesQueuedDeliveryOutsideOriginalCalendarDay(t *testing.T) {
	chatID := int64(10)
	billingDate := mustDate(t, 2026, time.September, 1)
	today := mustDate(t, 2026, time.September, 3)
	storage := &fakeStorage{
		reserved: true,
		attempt:  2,
		retryCandidates: []RetryCandidate{{
			User: domain.User{ID: 1, TelegramChatID: &chatID},
			Delivery: domain.ReminderDelivery{
				UserID: 1, BillingDate: billingDate, ScheduledDate: billingDate,
				ReminderType: domain.ReminderTypeManual, DeliveryKey: "update:42",
				MessageText: "custom reminder", Status: domain.ReminderStatusFailed,
			},
		}},
	}
	sender := &fakeSender{messageID: 7}
	service, _ := New(storage, sender, localization.Russian)
	service.now = func() time.Time { return time.Date(2026, time.September, 3, 9, 0, 0, 0, time.UTC) }

	delivered, err := service.Process(context.Background(), today)
	if err != nil || delivered != 1 || sender.calls != 1 {
		t.Fatalf("Process() = %d, %v, sender calls=%d", delivered, err, sender.calls)
	}
	if len(storage.reservations) != 1 || storage.reservations[0].DeliveryKey != "update:42" || storage.reservations[0].MessageText != "custom reminder" {
		t.Fatalf("reservation = %#v", storage.reservations)
	}
}

func TestConfirmUnknownNotSentUsesExplicitReconciliation(t *testing.T) {
	date := mustDate(t, 2026, time.September, 1)
	storage := &fakeStorage{unknownMarked: true}
	service, _ := New(storage, &fakeSender{}, localization.Russian)

	marked, err := service.ConfirmUnknownNotSent(context.Background(), 1, date, domain.ReminderTypeManual, "update:42")
	if err != nil || !marked || storage.unknownCalls != 1 || storage.unknownKey != "update:42" {
		t.Fatalf("ConfirmUnknownNotSent() = %t, %v, calls=%d, key=%q", marked, err, storage.unknownCalls, storage.unknownKey)
	}
}

type fakeStorage struct {
	reserved        bool
	attempt         int
	reservations    []domain.ReminderDelivery
	updated         []domain.ReminderDelivery
	retryAt         []*time.Time
	updateErr       error
	candidates      []Candidate
	candidateErr    error
	retryCandidates []RetryCandidate
	unknownMarked   bool
	unknownCalls    int
	unknownKey      string
}

func (f *fakeStorage) ReminderCandidates(context.Context, domain.Date) ([]Candidate, error) {
	return f.candidates, f.candidateErr
}
func (f *fakeStorage) RetryableReminderDeliveries(context.Context, time.Time, int, int) ([]RetryCandidate, error) {
	candidates := f.retryCandidates
	f.retryCandidates = nil
	return candidates, nil
}
func (f *fakeStorage) ReserveReminderDelivery(_ context.Context, d domain.ReminderDelivery, _ int) (domain.ReminderDelivery, int, bool, error) {
	f.reservations = append(f.reservations, d)
	attempt := f.attempt
	if attempt == 0 {
		attempt = 1
	}
	return d, attempt, f.reserved, nil
}
func (f *fakeStorage) UpdateReminderDeliveryAttempt(_ context.Context, d domain.ReminderDelivery, _ int, retryAt *time.Time) (bool, error) {
	f.updated = append(f.updated, d)
	f.retryAt = append(f.retryAt, retryAt)
	return f.updateErr == nil, f.updateErr
}
func (f *fakeStorage) MarkPendingUnknown(context.Context, time.Time) (int, error) { return 0, nil }
func (f *fakeStorage) MarkAllPendingUnknown(context.Context, time.Time) (int, error) {
	return 0, nil
}
func (f *fakeStorage) MarkUnknownRetryable(_ context.Context, _ domain.UserID, _ domain.Date, _ domain.ReminderType, deliveryKey string, _ time.Time, _ int) (bool, error) {
	f.unknownCalls++
	f.unknownKey = deliveryKey
	return f.unknownMarked, nil
}

type fakeSender struct {
	calls     int
	messageID int
	err       error
	errs      []error
	code      DeliveryErrorCode
}

func (f *fakeSender) SendReminder(context.Context, int64, string) (int, error) {
	f.calls++
	if f.calls <= len(f.errs) {
		return f.messageID, f.errs[f.calls-1]
	}
	return f.messageID, f.err
}
func (f *fakeSender) ClassifyReminderError(error) DeliveryErrorCode { return f.code }
