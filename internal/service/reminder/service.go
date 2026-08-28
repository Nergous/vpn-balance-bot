package reminder

import (
	"context"
	"errors"
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
