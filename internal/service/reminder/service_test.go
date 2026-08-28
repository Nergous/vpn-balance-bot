package reminder

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/Nergous/vpn-balance-bot/internal/domain"
)

func TestDeliverReservesBeforeSending(t *testing.T) {
	chatID := int64(10)
	now := time.Date(2026, time.August, 28, 0, 0, 0, 0, time.UTC)
	storage := &fakeStorage{created: true}
	sender := &fakeSender{messageID: 7}
	service, err := New(storage, sender)
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
	storage := &fakeStorage{created: false}
	sender := &fakeSender{}
	service, _ := New(storage, sender)
	created, err := service.Deliver(context.Background(), domain.User{ID: 1, TelegramChatID: &chatID}, date, date, domain.ReminderTypeManual, "test")
	if err != nil || created || sender.calls != 0 {
		t.Fatalf("duplicate = %t, %v, calls=%d", created, err, sender.calls)
	}

	storage.created = true
	sender.err = errors.New("network")
	created, err = service.Deliver(context.Background(), domain.User{ID: 1, TelegramChatID: &chatID}, date, date, domain.ReminderTypeManual, "test")
	if err != nil || created || len(storage.updated) != 1 || storage.updated[0].Status != domain.ReminderStatusFailed {
		t.Fatalf("failure = %t, %v, updates=%#v", created, err, storage.updated)
	}
}

type fakeStorage struct {
	created bool
	updated []domain.ReminderDelivery
}

func (f *fakeStorage) CreateReminderDelivery(_ context.Context, d domain.ReminderDelivery) (domain.ReminderDelivery, bool, error) {
	return d, f.created, nil
}
func (f *fakeStorage) UpdateReminderDelivery(_ context.Context, d domain.ReminderDelivery) error {
	f.updated = append(f.updated, d)
	return nil
}
func (f *fakeStorage) MarkPendingUnknown(context.Context, time.Time) (int, error) { return 0, nil }

type fakeSender struct {
	calls     int
	messageID int
	err       error
}

func (f *fakeSender) SendReminder(context.Context, int64, string) (int, error) {
	f.calls++
	return f.messageID, f.err
}
