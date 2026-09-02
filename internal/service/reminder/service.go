package reminder

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/Nergous/vpn-balance-bot/internal/domain"
	"github.com/Nergous/vpn-balance-bot/internal/localization"
)

var (
	ErrNilStorage = errors.New("reminder storage is nil")
	ErrNilSender  = errors.New("reminder sender is nil")
)

const maxDeliveryAttempts = 3

// Service selects, reserves, and delivers customer reminders.
type Service struct {
	storage  Storage
	sender   Sender
	now      func() time.Time
	language localization.Language
}

// New creates a reminder service for a supported localization catalog.
func New(storage Storage, sender Sender, language localization.Language) (*Service, error) {
	if storage == nil {
		return nil, ErrNilStorage
	}

	if sender == nil {
		return nil, ErrNilSender
	}

	if _, err := localization.New(language); err != nil {
		return nil, err
	}

	return &Service{
		storage:  storage,
		sender:   sender,
		now:      time.Now,
		language: language,
	}, nil
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

	candidates, err := s.storage.ReminderCandidates(ctx, today)
	if err != nil {
		return 0, fmt.Errorf("list reminder candidates: %w", err)
	}

	delivered := 0
	var processErrors []error

	for _, candidate := range candidates {
		reminderType, billingDate, ok := SelectAutomatic(candidate, today)
		if !ok {
			continue
		}

		created, err := s.Deliver(ctx, candidate.User, billingDate, today, reminderType, automaticText(s.language, reminderType))
		if err != nil {
			processErrors = append(processErrors, fmt.Errorf("deliver %s reminder to user %d: %w", reminderType, candidate.User.ID, err))
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
	delivery, attempt, reserved, err := s.storage.ReserveReminderDelivery(ctx, domain.ReminderDelivery{
		UserID:        user.ID,
		BillingDate:   billingDate,
		ScheduledDate: scheduledDate,
		ReminderType:  reminderType,
		Status:        domain.ReminderStatusPending,
		CreatedAt:     now,
		UpdatedAt:     now,
	}, maxDeliveryAttempts)
	if err != nil || !reserved {
		return false, err
	}

	messageID, err := s.sender.SendReminder(ctx, *user.TelegramChatID, text)
	if err != nil {
		classification := classifyDeliveryError(s.sender, err)
		code := string(classification)
		delivery.Status = domain.ReminderStatusFailed
		delivery.ErrorCode = &code
		delivery.UpdatedAt = s.now().UTC().Truncate(time.Second)
		var retryAt *time.Time
		if classification == DeliveryErrorRetryable && attempt < maxDeliveryAttempts {
			nextAttempt := delivery.UpdatedAt.Add(deliveryRetryBackoff(attempt))
			retryAt = &nextAttempt
		}

		updateErr := s.storage.UpdateReminderDeliveryAttempt(ctx, delivery, retryAt)
		resultErr := err
		if retryAt != nil {
			resultErr = errors.Join(resultErr, ErrRetryableDelivery)
		}
		if updateErr != nil {
			resultErr = errors.Join(resultErr, updateErr)
		}

		return false, resultErr
	}

	delivery.Status = domain.ReminderStatusSent
	delivery.SentAt = &now
	telegramMessageID := int64(messageID)
	delivery.TelegramMessageID = &telegramMessageID
	delivery.UpdatedAt = now
	if err := s.storage.UpdateReminderDeliveryAttempt(ctx, delivery, nil); err != nil {
		return false, err
	}

	return true, nil
}

// ConfirmUnknownNotSent makes an ambiguous delivery retryable only after an
// operator or reconciliation process has confirmed that no message was sent.
func (s *Service) ConfirmUnknownNotSent(ctx context.Context, userID domain.UserID, billingDate domain.Date, reminderType domain.ReminderType) (bool, error) {
	return s.storage.MarkUnknownRetryable(
		ctx,
		userID,
		billingDate,
		reminderType,
		s.now().UTC().Truncate(time.Second),
		maxDeliveryAttempts,
	)
}

func deliveryRetryBackoff(attempt int) time.Duration {
	delay := time.Minute << max(attempt-1, 0)
	return min(delay, 15*time.Minute)
}

func automaticText(language localization.Language, reminderType domain.ReminderType) string {
	localizer := localization.MustNew(language)
	switch reminderType {
	case domain.ReminderTypeBeforeCharge:
		return localizer.Text("ReminderBefore", nil)
	case domain.ReminderTypeChargeDebt:
		return localizer.Text("ReminderCharge", nil)
	case domain.ReminderTypeOverdue3D:
		return localizer.Text("ReminderOverdue3", nil)
	case domain.ReminderTypeOverdue7D:
		return localizer.Text("ReminderOverdue7", nil)
	default:
		return localizer.Text("ReminderDefault", nil)
	}
}
