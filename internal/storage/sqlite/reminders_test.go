package sqlite

import (
	"context"
	"testing"
	"time"

	"github.com/Nergous/vpn-balance-bot/internal/domain"
	"github.com/Nergous/vpn-balance-bot/internal/service/reminder"
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

func TestReminderDeliveryRetryAndAttemptFencing(t *testing.T) {
	ctx := context.Background()
	store := newTestSQLite(t, ctx)
	if err := store.Migrate(ctx); err != nil {
		t.Fatal(err)
	}
	date, _ := domain.NewDate(2026, time.September, 2)
	now := time.Date(2026, time.September, 2, 10, 0, 0, 0, time.UTC)
	user, err := store.CreateUser(ctx, domain.User{DisplayName: "Retry", MonthlyFeeMinor: 100, Currency: "RUB", BillingAnchorDay: 2, NextChargeOn: date, Status: domain.UserStatusActive, CreatedAt: now, UpdatedAt: now})
	if err != nil {
		t.Fatal(err)
	}
	lease := now.Add(time.Minute)
	delivery := domain.ReminderDelivery{UserID: user.ID, BillingDate: date, ScheduledDate: date, ReminderType: domain.ReminderTypeManual, DeliveryKey: "update:42", Status: domain.ReminderStatusPending, CreatedAt: now, UpdatedAt: now, LeaseExpiresAt: &lease}
	reserved, attempt, created, err := store.ReserveReminderDelivery(ctx, delivery, 3)
	if err != nil || !created || attempt != 1 {
		t.Fatalf("reserve = %#v, %d, %t, %v", reserved, attempt, created, err)
	}
	code := string(reminder.DeliveryErrorRetryable)
	delivery.Status = domain.ReminderStatusFailed
	delivery.ErrorCode = &code
	retryAt := now.Add(time.Minute)
	if updated, err := store.UpdateReminderDeliveryAttempt(ctx, delivery, 1, &retryAt); err != nil || !updated {
		t.Fatalf("failure update = %t, %v", updated, err)
	}
	delivery.UpdatedAt = now.Add(30 * time.Second)
	if _, _, created, err := store.ReserveReminderDelivery(ctx, delivery, 3); err != nil || created {
		t.Fatalf("early retry = %t, %v", created, err)
	}
	delivery.Status = domain.ReminderStatusPending
	delivery.UpdatedAt = retryAt
	lease2 := retryAt.Add(time.Minute)
	delivery.LeaseExpiresAt = &lease2
	_, attempt, created, err = store.ReserveReminderDelivery(ctx, delivery, 3)
	if err != nil || !created || attempt != 2 {
		t.Fatalf("due retry = %d, %t, %v", attempt, created, err)
	}
	delivery.Status = domain.ReminderStatusSent
	if updated, err := store.UpdateReminderDeliveryAttempt(ctx, delivery, 1, nil); err != nil || updated {
		t.Fatalf("stale attempt update = %t, %v", updated, err)
	}
	if recovered, err := store.MarkPendingUnknown(ctx, retryAt.Add(30*time.Second)); err != nil || recovered != 0 {
		t.Fatalf("active lease recovery = %d, %v", recovered, err)
	}
	if recovered, err := store.MarkPendingUnknown(ctx, lease2); err != nil || recovered != 1 {
		t.Fatalf("expired lease recovery = %d, %v", recovered, err)
	}
}

func TestMarkAllPendingUnknownRecoversUnexpiredLeaseAtStartup(t *testing.T) {
	ctx := context.Background()
	store := newTestSQLite(t, ctx)
	if err := store.Migrate(ctx); err != nil {
		t.Fatal(err)
	}
	date, _ := domain.NewDate(2026, time.September, 3)
	now := time.Date(2026, time.September, 3, 10, 0, 0, 0, time.UTC)
	user, err := store.CreateUser(ctx, domain.User{
		DisplayName: "Restart", MonthlyFeeMinor: 100, Currency: "RUB",
		BillingAnchorDay: 3, NextChargeOn: date, Status: domain.UserStatusActive,
		CreatedAt: now, UpdatedAt: now,
	})
	if err != nil {
		t.Fatal(err)
	}
	lease := now.Add(2 * time.Minute)
	delivery := domain.ReminderDelivery{
		UserID: user.ID, BillingDate: date, ScheduledDate: date,
		ReminderType: domain.ReminderTypeManual, DeliveryKey: "update:restart",
		Status: domain.ReminderStatusPending, CreatedAt: now, UpdatedAt: now,
		LeaseExpiresAt: &lease,
	}
	if _, _, reserved, err := store.ReserveReminderDelivery(ctx, delivery, 3); err != nil || !reserved {
		t.Fatalf("reserve = %t, %v", reserved, err)
	}

	recoveredAt := now.Add(30 * time.Second)
	recovered, err := store.MarkAllPendingUnknown(ctx, recoveredAt)
	if err != nil || recovered != 1 {
		t.Fatalf("MarkAllPendingUnknown() = %d, %v", recovered, err)
	}
	stored, _, err := store.reminderDeliveryAttempt(ctx, user.ID, date, domain.ReminderTypeManual, "update:restart")
	if err != nil {
		t.Fatal(err)
	}
	if stored.Status != domain.ReminderStatusFailed || stored.ErrorCode == nil ||
		*stored.ErrorCode != string(reminder.DeliveryErrorUnknown) || stored.LeaseExpiresAt != nil {
		t.Fatalf("recovered delivery = %#v", stored)
	}
}
