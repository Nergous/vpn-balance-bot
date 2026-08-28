package sqlite

import (
	"context"
	"testing"
	"time"

	"github.com/Nergous/vpn-balance-bot/internal/domain"
)

func TestReminderDeliveryReservationAndUpdate(t *testing.T) {
	ctx := context.Background()
	store := newTestSQLite(t, ctx)
	if err := store.Migrate(ctx); err != nil {
		t.Fatal(err)
	}
	date, _ := domain.NewDate(2026, time.September, 1)
	now := time.Date(2026, time.August, 28, 0, 0, 0, 0, time.UTC)
	user, err := store.CreateUser(ctx, domain.User{DisplayName: "Reminder", MonthlyFeeMinor: 100, Currency: "RUB", BillingAnchorDay: 1, NextChargeOn: date, Status: domain.UserStatusActive, CreatedAt: now, UpdatedAt: now})
	if err != nil {
		t.Fatal(err)
	}
	d := domain.ReminderDelivery{UserID: user.ID, BillingDate: date, ScheduledDate: date, ReminderType: domain.ReminderTypeManual, Status: domain.ReminderStatusPending, CreatedAt: now, UpdatedAt: now}
	initial := d
	_, created, err := store.CreateReminderDelivery(ctx, d)
	if err != nil || !created {
		t.Fatalf("first=%t,%v", created, err)
	}
	existing, created, err := store.CreateReminderDelivery(ctx, initial)
	if err != nil || created || existing.Status != domain.ReminderStatusPending {
		t.Fatalf("duplicate=%#v,%t,%v", existing, created, err)
	}
	messageID := int64(5)
	sentAt := now.Add(time.Second)
	d.Status = domain.ReminderStatusSent
	d.SentAt = &sentAt
	d.TelegramMessageID = &messageID
	d.UpdatedAt = sentAt
	if err := store.UpdateReminderDelivery(ctx, d); err != nil {
		t.Fatal(err)
	}
	existing, created, err = store.CreateReminderDelivery(ctx, initial)
	if err != nil || created || existing.Status != domain.ReminderStatusSent || existing.TelegramMessageID == nil || *existing.TelegramMessageID != 5 {
		t.Fatalf("updated=%#v,%t,%v", existing, created, err)
	}
}
