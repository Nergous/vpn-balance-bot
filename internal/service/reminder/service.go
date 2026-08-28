package reminder

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/Nergous/vpn-balance-bot/internal/domain"
)

var (
	ErrNilStorage = errors.New("reminder storage is nil")
	ErrNilSender  = errors.New("reminder sender is nil")
)

type Service struct {
	storage Storage
	sender  Sender
	now     func() time.Time
}

func New(storage Storage, sender Sender) (*Service, error) {
	if storage == nil {
		return nil, ErrNilStorage
	}
	if sender == nil {
		return nil, ErrNilSender
	}
	return &Service{storage: storage, sender: sender, now: time.Now}, nil
}

// RecoverPending marks deliveries left pending by a prior process as unknown.
func (s *Service) RecoverPending(ctx context.Context) (int, error) {
	return s.storage.MarkPendingUnknown(ctx, s.now().UTC().Truncate(time.Second))
}

// Process recovers interrupted deliveries and sends the one automatic reminder
// applicable to each active user for today. Delivery reservation makes retries safe.
func (s *Service) Process(ctx context.Context, today domain.Date) (int, error) {
	if !today.IsValid() {
		return 0, domain.ErrInvalidDate
	}
	if _, err := s.RecoverPending(ctx); err != nil {
		return 0, fmt.Errorf("recover pending reminder deliveries: %w", err)
	}

	status := domain.UserStatusActive
	users, err := s.storage.ListUsers(ctx, &status)
	if err != nil {
		return 0, fmt.Errorf("list active reminder users: %w", err)
	}

	delivered := 0
	for _, user := range users {
		balance, err := s.storage.Balance(ctx, user.ID)
		if err != nil {
			return delivered, fmt.Errorf("get balance for reminder user %d: %w", user.ID, err)
		}
		reminderType, ok := SelectAutomatic(user, balance, today)
		if !ok {
			continue
		}
		created, err := s.Deliver(ctx, user, user.NextChargeOn, today, reminderType, automaticText(reminderType))
		if err != nil {
			return delivered, fmt.Errorf("deliver %s reminder to user %d: %w", reminderType, user.ID, err)
		}
		if created {
			delivered++
		}
	}
	return delivered, nil
}

// Deliver reserves exactly one delivery key before sending it.
func (s *Service) Deliver(ctx context.Context, user domain.User, billingDate, scheduledDate domain.Date, reminderType domain.ReminderType, text string) (bool, error) {
	if user.TelegramChatID == nil {
		return false, nil
	}

	now := s.now().UTC().Truncate(time.Second)
	delivery, created, err := s.storage.CreateReminderDelivery(ctx, domain.ReminderDelivery{
		UserID: user.ID, BillingDate: billingDate, ScheduledDate: scheduledDate,
		ReminderType: reminderType, Status: domain.ReminderStatusPending,
		CreatedAt: now, UpdatedAt: now,
	})
	if err != nil || !created {
		return false, err
	}

	messageID, err := s.sender.SendReminder(ctx, *user.TelegramChatID, text)
	if err != nil {
		code := "delivery_failed"
		delivery.Status = domain.ReminderStatusFailed
		delivery.ErrorCode = &code
		delivery.UpdatedAt = s.now().UTC().Truncate(time.Second)
		return false, s.storage.UpdateReminderDelivery(ctx, delivery)
	}

	delivery.Status = domain.ReminderStatusSent
	delivery.SentAt = &now
	telegramMessageID := int64(messageID)
	delivery.TelegramMessageID = &telegramMessageID
	delivery.UpdatedAt = now
	if err := s.storage.UpdateReminderDelivery(ctx, delivery); err != nil {
		return false, err
	}
	return true, nil
}

func automaticText(reminderType domain.ReminderType) string {
	switch reminderType {
	case domain.ReminderTypeBeforeCharge:
		return "Your subscription charge is due in 3 days."
	case domain.ReminderTypeChargeDebt:
		return "Your subscription charge was applied. Please top up your balance."
	case domain.ReminderTypeOverdue3D:
		return "Your balance has been overdue for 3 days. Please top up your balance."
	case domain.ReminderTypeOverdue7D:
		return "Your balance has been overdue for 7 days. Please top up your balance."
	default:
		return "Please top up your balance."
	}
}
