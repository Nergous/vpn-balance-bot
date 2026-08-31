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
	storage  Storage
	sender   Sender
	now      func() time.Time
	language string
}

func New(storage Storage, sender Sender, language ...string) (*Service, error) {
	if storage == nil {
		return nil, ErrNilStorage
	}
	if sender == nil {
		return nil, ErrNilSender
	}
	selected := "ru"
	if len(language) > 0 && language[0] == "en" {
		selected = "en"
	}
	return &Service{storage: storage, sender: sender, now: time.Now, language: selected}, nil
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
	var processErrors []error
	for _, user := range users {
		balance, err := s.storage.Balance(ctx, user.ID)
		if err != nil {
			processErrors = append(processErrors, fmt.Errorf("get balance for reminder user %d: %w", user.ID, err))
			continue
		}
		reminderType, ok := SelectAutomatic(user, balance, today)
		if !ok {
			continue
		}
		created, err := s.Deliver(ctx, user, user.NextChargeOn, today, reminderType, automaticText(s.language, reminderType))
		if err != nil {
			processErrors = append(processErrors, fmt.Errorf("deliver %s reminder to user %d: %w", reminderType, user.ID, err))
			continue
		}
		if created {
			delivered++
		}
	}
	return delivered, errors.Join(processErrors...)
}

// DeliverManual sends an explicit admin-requested reminder through the same
// reservation and result-recording path as automatic deliveries.
func (s *Service) DeliverManual(ctx context.Context, user domain.User, text string) (bool, error) {
	today, err := domain.DateFromTime(s.now().UTC(), time.UTC)
	if err != nil {
		return false, err
	}
	return s.Deliver(ctx, user, user.NextChargeOn, today, domain.ReminderTypeManual, text)
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
		code := string(classifyDeliveryError(s.sender, err))
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

func automaticText(language string, reminderType domain.ReminderType) string {
	english := language == "en"
	switch reminderType {
	case domain.ReminderTypeBeforeCharge:
		if !english {
			return "Списание за подписку будет через 3 дня."
		}
		return "Your subscription charge is due in 3 days."
	case domain.ReminderTypeChargeDebt:
		if !english {
			return "Подписка списана. Пополните баланс."
		}
		return "Your subscription charge was applied. Please top up your balance."
	case domain.ReminderTypeOverdue3D:
		if !english {
			return "Долг сохраняется уже 3 дня. Пополните баланс."
		}
		return "Your balance has been overdue for 3 days. Please top up your balance."
	case domain.ReminderTypeOverdue7D:
		if !english {
			return "Долг сохраняется уже 7 дней. Пополните баланс."
		}
		return "Your balance has been overdue for 7 days. Please top up your balance."
	default:
		if !english {
			return "Пополните баланс."
		}
		return "Please top up your balance."
	}
}
