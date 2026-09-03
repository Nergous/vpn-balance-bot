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
	ErrNilStorage           = errors.New("reminder storage is nil")
	ErrNilSender            = errors.New("reminder sender is nil")
	ErrStaleDeliveryAttempt = errors.New("reminder delivery attempt is stale")
)

const (
	maxDeliveryAttempts = 3
	deliveryLease       = 2 * time.Minute
	candidatePageSize   = 100
	deliveryWorkers     = 4
)

type pagedStorage interface {
	ReminderCandidatesPage(context.Context, domain.Date, domain.UserID, int) ([]Candidate, bool, error)
}

type retryStorage interface {
	RetryableReminderDeliveries(context.Context, time.Time, int, int) ([]RetryCandidate, error)
}

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

// RecoverPending marks every delivery left pending by a prior process as
// unknown. The application is intentionally single-process, so no active
// delivery can belong to another live owner during startup.
func (s *Service) RecoverPending(ctx context.Context) (int, error) {
	return s.storage.MarkAllPendingUnknown(ctx, s.now().UTC().Truncate(time.Second))
}

// Process sends the one automatic reminder applicable to each active user for today.
func (s *Service) Process(ctx context.Context, today domain.Date) (int, error) {
	if !today.IsValid() {
		return 0, domain.ErrInvalidDate
	}
	if _, err := s.storage.MarkPendingUnknown(ctx, s.now().UTC().Truncate(time.Second)); err != nil {
		return 0, fmt.Errorf("recover expired reminder deliveries: %w", err)
	}

	delivered := 0
	var processErrors []error
	retryCount, retryErrors := s.processRetryableDeliveries(ctx)
	delivered += retryCount
	processErrors = append(processErrors, retryErrors...)
	if storage, ok := s.storage.(pagedStorage); ok {
		var afterID domain.UserID
		for {
			candidates, hasMore, err := storage.ReminderCandidatesPage(ctx, today, afterID, candidatePageSize)
			if err != nil {
				return delivered, fmt.Errorf("list reminder candidates: %w", err)
			}
			count, errs := s.processCandidates(ctx, today, candidates, deliveryWorkers)
			delivered += count
			processErrors = append(processErrors, errs...)
			if !hasMore || len(candidates) == 0 {
				break
			}
			afterID = candidates[len(candidates)-1].User.ID
		}
	} else {
		candidates, err := s.storage.ReminderCandidates(ctx, today)
		if err != nil {
			return 0, fmt.Errorf("list reminder candidates: %w", err)
		}
		count, errs := s.processCandidates(ctx, today, candidates, 1)
		delivered += count
		processErrors = append(processErrors, errs...)
	}

	return delivered, errors.Join(processErrors...)
}

func (s *Service) processRetryableDeliveries(ctx context.Context) (int, []error) {
	storage, ok := s.storage.(retryStorage)
	if !ok {
		return 0, nil
	}

	delivered := 0
	var processErrors []error
	for {
		now := s.now().UTC().Truncate(time.Second)
		candidates, err := storage.RetryableReminderDeliveries(ctx, now, maxDeliveryAttempts, candidatePageSize)
		if err != nil {
			return delivered, append(processErrors, fmt.Errorf("list retryable reminder deliveries: %w", err))
		}
		if len(candidates) == 0 {
			return delivered, processErrors
		}

		results := make(chan deliveryResult, len(candidates))
		jobs := make(chan RetryCandidate)
		workers := min(deliveryWorkers, len(candidates))
		for range workers {
			go func() {
				for candidate := range jobs {
					delivery := candidate.Delivery
					text := delivery.MessageText
					if text == "" {
						text = automaticText(s.language, delivery.ReminderType)
					}
					created, err := s.deliver(ctx, candidate.User, delivery.BillingDate, delivery.ScheduledDate, delivery.ReminderType, delivery.DeliveryKey, text)
					if err != nil {
						err = fmt.Errorf("retry %s reminder for user %d key %q: %w", delivery.ReminderType, candidate.User.ID, delivery.DeliveryKey, err)
					}
					results <- deliveryResult{delivered: created, err: err}
				}
			}()
		}
		for _, candidate := range candidates {
			jobs <- candidate
		}
		close(jobs)
		for range candidates {
			result := <-results
			if result.delivered {
				delivered++
			}
			if result.err != nil {
				processErrors = append(processErrors, result.err)
			}
		}
		if len(candidates) < candidatePageSize {
			return delivered, processErrors
		}
	}
}

type deliveryResult struct {
	delivered bool
	err       error
}

func (s *Service) processCandidates(ctx context.Context, today domain.Date, candidates []Candidate, maxWorkers int) (int, []error) {
	jobs := make(chan Candidate)
	results := make(chan deliveryResult, len(candidates))
	workers := min(maxWorkers, len(candidates))
	for range workers {
		go func() {
			for candidate := range jobs {
				reminderType, billingDate, ok := SelectAutomatic(candidate, today)
				if !ok {
					results <- deliveryResult{}
					continue
				}
				created, err := s.Deliver(ctx, candidate.User, billingDate, today, reminderType, automaticText(s.language, reminderType))
				if err != nil {
					err = fmt.Errorf("deliver %s reminder to user %d: %w", reminderType, candidate.User.ID, err)
				}
				results <- deliveryResult{delivered: created, err: err}
			}
		}()
	}
	for _, candidate := range candidates {
		jobs <- candidate
	}
	close(jobs)
	delivered := 0
	var processErrors []error
	for range candidates {
		result := <-results
		if result.delivered {
			delivered++
		}
		if result.err != nil {
			processErrors = append(processErrors, result.err)
		}
	}
	return delivered, processErrors
}

// DeliverManual sends an explicit admin-requested reminder through the same
// reservation and result-recording path as automatic deliveries.
func (s *Service) DeliverManual(ctx context.Context, user domain.User, updateID int64, text string) (bool, error) {
	today, err := domain.DateFromTime(s.now().UTC(), time.UTC)
	if err != nil {
		return false, err
	}

	return s.deliver(ctx, user, user.NextChargeOn, today, domain.ReminderTypeManual, fmt.Sprintf("update:%d", updateID), text)
}

// Deliver reserves exactly one delivery key before sending it.
func (s *Service) Deliver(ctx context.Context, user domain.User, billingDate, scheduledDate domain.Date, reminderType domain.ReminderType, text string) (bool, error) {
	return s.deliver(ctx, user, billingDate, scheduledDate, reminderType, "", text)
}

func (s *Service) deliver(ctx context.Context, user domain.User, billingDate, scheduledDate domain.Date, reminderType domain.ReminderType, deliveryKey, text string) (bool, error) {
	if user.TelegramChatID == nil {
		return false, nil
	}

	now := s.now().UTC().Truncate(time.Second)
	leaseExpiresAt := now.Add(deliveryLease)
	delivery, attempt, reserved, err := s.storage.ReserveReminderDelivery(ctx, domain.ReminderDelivery{
		UserID:         user.ID,
		BillingDate:    billingDate,
		ScheduledDate:  scheduledDate,
		ReminderType:   reminderType,
		DeliveryKey:    deliveryKey,
		MessageText:    text,
		Status:         domain.ReminderStatusPending,
		CreatedAt:      now,
		UpdatedAt:      now,
		LeaseExpiresAt: &leaseExpiresAt,
	}, maxDeliveryAttempts)
	if err != nil {
		return false, err
	}
	if !reserved {
		if delivery.Status == domain.ReminderStatusFailed && delivery.ErrorCode != nil &&
			*delivery.ErrorCode == string(DeliveryErrorRetryable) && attempt < maxDeliveryAttempts {
			return false, ErrRetryableDelivery
		}
		return false, nil
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

		updated, updateErr := s.storage.UpdateReminderDeliveryAttempt(ctx, delivery, attempt, retryAt)
		if updateErr == nil && !updated {
			updateErr = ErrStaleDeliveryAttempt
		}
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
	updated, err := s.storage.UpdateReminderDeliveryAttempt(ctx, delivery, attempt, nil)
	if err != nil {
		return false, err
	}
	if !updated {
		return false, ErrStaleDeliveryAttempt
	}

	return true, nil
}

// ConfirmUnknownNotSent makes an ambiguous delivery retryable only after an
// operator or reconciliation process has confirmed that no message was sent.
func (s *Service) ConfirmUnknownNotSent(ctx context.Context, userID domain.UserID, billingDate domain.Date, reminderType domain.ReminderType, deliveryKey string) (bool, error) {
	return s.storage.MarkUnknownRetryable(
		ctx,
		userID,
		billingDate,
		reminderType,
		deliveryKey,
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
