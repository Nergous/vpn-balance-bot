package account

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/Nergous/vpn-balance-bot/internal/domain"
)

const supportedCurrency = "RUB"

// Service manages customer billing profiles.
type Service struct {
	storage   Storage
	inviteTTL time.Duration
	now       func() time.Time
}

// New creates an account service backed by storage.
func New(storage Storage, inviteTTL time.Duration) (*Service, error) {
	if storage == nil {
		return nil, ErrNilStorage
	}

	if inviteTTL < time.Second {
		return nil, ErrInvalidInviteTTL
	}

	return newService(storage, inviteTTL, time.Now), nil
}

func newService(storage Storage, inviteTTL time.Duration, now func() time.Time) *Service {
	return &Service{
		storage:   storage,
		inviteTTL: inviteTTL,
		now:       now,
	}
}

// CreateUser validates and creates an active customer billing profile.
func (s *Service) CreateUser(ctx context.Context, params CreateUserParams) (domain.User, error) {
	if err := validateAdminTelegramID(params.AdminTelegramID); err != nil {
		return domain.User{}, err
	}
	displayName, err := validateCreateUserParams(params)

	if err != nil {
		return domain.User{}, err
	}

	now := s.now().UTC().Truncate(time.Second)

	return s.storage.CreateUser(ctx, domain.User{
		TelegramUserID:   params.TelegramUserID,
		TelegramChatID:   params.TelegramChatID,
		Username:         params.Username,
		DisplayName:      displayName,
		MonthlyFeeMinor:  params.MonthlyFeeMinor,
		Currency:         params.Currency,
		BillingAnchorDay: params.BillingAnchorDay,
		NextChargeOn:     params.NextChargeOn,
		Status:           domain.UserStatusActive,
		CreatedAt:        now,
		UpdatedAt:        now,
	})
}

// UserByID returns a customer billing profile by its internal identifier.
func (s *Service) UserByID(ctx context.Context, userID domain.UserID) (domain.User, error) {
	return s.storage.UserByID(ctx, userID)
}

// UserByTelegramID returns a customer billing profile linked to a Telegram user.
func (s *Service) UserByTelegramID(ctx context.Context, telegramID int64) (domain.User, error) {
	return s.storage.UserByTelegramID(ctx, telegramID)
}

// ListUsers returns customer profiles matching filter.
func (s *Service) ListUsers(ctx context.Context, filter UserFilter) ([]domain.User, error) {
	if filter.Status != nil && !filter.Status.IsValid() {
		return nil, ErrInvalidUserStatus
	}

	return s.storage.ListUsers(ctx, filter.Status)
}

type userPageStorage interface {
	ListUsersPage(context.Context, *domain.UserStatus, domain.UserID, int) ([]domain.User, bool, error)
}

// ListUsersPage returns a bounded page ordered by user ID.
func (s *Service) ListUsersPage(ctx context.Context, filter UserFilter, afterID domain.UserID, limit int) ([]domain.User, bool, error) {
	if filter.Status != nil && !filter.Status.IsValid() {
		return nil, false, ErrInvalidUserStatus
	}
	if limit <= 0 {
		return nil, false, ErrInvalidUserPageLimit
	}
	if storage, ok := s.storage.(userPageStorage); ok {
		return storage.ListUsersPage(ctx, filter.Status, afterID, limit)
	}

	users, err := s.storage.ListUsers(ctx, filter.Status)
	if err != nil {
		return nil, false, err
	}
	page := make([]domain.User, 0, limit)
	for _, user := range users {
		if user.ID <= afterID {
			continue
		}
		if len(page) == limit {
			return page, true, nil
		}
		page = append(page, user)
	}
	return page, false, nil
}

type userStatusCountStorage interface {
	UserStatusCounts(context.Context) (UserStatusCounts, error)
}

// UserStatusCounts returns aggregate profile counts without materializing users.
func (s *Service) UserStatusCounts(ctx context.Context) (UserStatusCounts, error) {
	if storage, ok := s.storage.(userStatusCountStorage); ok {
		return storage.UserStatusCounts(ctx)
	}
	users, err := s.storage.ListUsers(ctx, nil)
	if err != nil {
		return UserStatusCounts{}, err
	}
	counts := UserStatusCounts{Total: len(users)}
	for _, user := range users {
		switch user.Status {
		case domain.UserStatusActive:
			counts.Active++
		case domain.UserStatusPaused:
			counts.Paused++
		case domain.UserStatusDisabled:
			counts.Disabled++
		}
	}
	return counts, nil
}

// ChangeMonthlyFee updates a customer's recurring monthly charge.
func (s *Service) ChangeMonthlyFee(ctx context.Context, params ChangeMonthlyFeeParams) (domain.User, error) {
	if err := validateAdminTelegramID(params.AdminTelegramID); err != nil {
		return domain.User{}, err
	}
	if params.MonthlyFeeMinor <= 0 {
		return domain.User{}, ErrInvalidMonthlyFee
	}

	return s.storage.SetMonthlyFee(ctx, params.UserID, params.MonthlyFeeMinor, s.nowUTC())
}

// Pause suspends automatic billing for a customer profile.
func (s *Service) Pause(ctx context.Context, params AdminUserParams) (domain.User, error) {
	if err := validateAdminTelegramID(params.AdminTelegramID); err != nil {
		return domain.User{}, err
	}
	return s.storage.PauseUser(ctx, params.UserID, s.nowUTC())
}

// Resume reactivates a profile from the supplied next billing date.
func (s *Service) Resume(ctx context.Context, params ResumeParams) (domain.User, error) {
	if err := validateAdminTelegramID(params.AdminTelegramID); err != nil {
		return domain.User{}, err
	}
	if params.NextChargeOn == nil {
		return domain.User{}, ErrResumeDateRequired
	}
	if !params.NextChargeOn.IsValid() {
		return domain.User{}, ErrInvalidNextChargeOn
	}

	return s.storage.ResumeUser(ctx, params.UserID, *params.NextChargeOn, s.nowUTC())
}

// Disable permanently removes a customer profile from active billing.
func (s *Service) Disable(ctx context.Context, params AdminUserParams) (domain.User, error) {
	if err := validateAdminTelegramID(params.AdminTelegramID); err != nil {
		return domain.User{}, err
	}
	return s.storage.DisableUser(ctx, params.UserID, s.nowUTC())
}

// SavePaymentDraft persists an administrator payment draft when supported by storage.
func (s *Service) SavePaymentDraft(ctx context.Context, adminID int64, userID domain.UserID, amount domain.AmountMinor, note *string, occurredAt time.Time) (time.Time, error) {
	storage, ok := s.storage.(PaymentDraftStorage)
	if !ok {
		return time.Time{}, errors.New("payment draft storage is unavailable")
	}
	if occurredAt.IsZero() {
		occurredAt = s.nowUTC()
	} else {
		occurredAt = occurredAt.UTC()
	}
	err := storage.SavePaymentDraft(ctx, PaymentDraftRecord{
		AdminTelegramID: adminID,
		UserID:          userID,
		AmountMinor:     amount,
		Note:            note,
		UpdatedAt:       occurredAt,
	})
	return occurredAt, err
}

// PaymentDraft loads a durable administrator payment draft.
func (s *Service) PaymentDraft(ctx context.Context, adminID int64) (domain.UserID, domain.AmountMinor, *string, time.Time, bool, error) {
	storage, ok := s.storage.(PaymentDraftStorage)
	if !ok {
		return 0, 0, nil, time.Time{}, false, errors.New("payment draft storage is unavailable")
	}
	draft, found, err := storage.PaymentDraft(ctx, adminID)
	return draft.UserID, draft.AmountMinor, draft.Note, draft.UpdatedAt, found, err
}

// DeletePaymentDraft removes a confirmed or cancelled administrator draft.
func (s *Service) DeletePaymentDraft(ctx context.Context, adminID int64) error {
	storage, ok := s.storage.(PaymentDraftStorage)
	if !ok {
		return errors.New("payment draft storage is unavailable")
	}
	return storage.DeletePaymentDraft(ctx, adminID)
}

type telegramUpdateStorage interface {
	ProcessTelegramUpdate(context.Context, int64, func(context.Context) error) (bool, error)
}

// ProcessTelegramUpdate atomically deduplicates one state-changing Telegram update.
func (s *Service) ProcessTelegramUpdate(ctx context.Context, updateID int64, handler func(context.Context) error) (bool, error) {
	storage, ok := s.storage.(telegramUpdateStorage)
	if !ok {
		return false, errors.New("telegram update storage is unavailable")
	}
	return storage.ProcessTelegramUpdate(ctx, updateID, handler)
}

func (s *Service) nowUTC() time.Time {
	return s.now().UTC().Truncate(time.Second)
}

func validateAdminTelegramID(adminTelegramID int64) error {
	if adminTelegramID <= 0 {
		return ErrInvalidAdminTelegramID
	}
	return nil
}

func validateCreateUserParams(params CreateUserParams) (string, error) {
	displayName := strings.TrimSpace(params.DisplayName)
	if displayName == "" {
		return "", ErrInvalidDisplayName
	}

	if params.MonthlyFeeMinor <= 0 {
		return "", ErrInvalidMonthlyFee
	}

	if params.Currency != supportedCurrency {
		return "", fmt.Errorf("%w: %s", ErrUnsupportedCurrency, params.Currency)
	}

	if params.BillingAnchorDay < 1 || params.BillingAnchorDay > 31 {
		return "", ErrInvalidAnchorDay
	}

	if !params.NextChargeOn.IsValid() {
		return "", ErrInvalidNextChargeOn
	}

	return displayName, nil
}
