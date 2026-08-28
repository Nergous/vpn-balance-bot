package account

import (
	"context"
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
	if inviteTTL <= 0 {
		return nil, ErrInvalidInviteTTL
	}

	return newService(storage, inviteTTL, time.Now), nil
}

func newService(storage Storage, inviteTTL time.Duration, now func() time.Time) *Service {
	return &Service{storage: storage, inviteTTL: inviteTTL, now: now}
}

func (s *Service) CreateUser(ctx context.Context, params CreateUserParams) (domain.User, error) {
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

func (s *Service) UserByID(ctx context.Context, userID domain.UserID) (domain.User, error) {
	return s.storage.UserByID(ctx, userID)
}

func (s *Service) UserByTelegramID(ctx context.Context, telegramID int64) (domain.User, error) {
	return s.storage.UserByTelegramID(ctx, telegramID)
}

func (s *Service) ListUsers(ctx context.Context, filter UserFilter) ([]domain.User, error) {
	if filter.Status != nil && !filter.Status.IsValid() {
		return nil, ErrInvalidUserStatus
	}

	return s.storage.ListUsers(ctx, filter.Status)
}

func (s *Service) ChangeMonthlyFee(ctx context.Context, params ChangeMonthlyFeeParams) (domain.User, error) {
	if params.MonthlyFeeMinor <= 0 {
		return domain.User{}, ErrInvalidMonthlyFee
	}

	return s.storage.SetMonthlyFee(ctx, params.UserID, params.MonthlyFeeMinor, s.nowUTC())
}

func (s *Service) Pause(ctx context.Context, userID domain.UserID) (domain.User, error) {
	return s.storage.PauseUser(ctx, userID, s.nowUTC())
}

func (s *Service) Resume(ctx context.Context, params ResumeParams) (domain.User, error) {
	if params.NextChargeOn == nil {
		return domain.User{}, ErrResumeDateRequired
	}
	if !params.NextChargeOn.IsValid() {
		return domain.User{}, ErrInvalidNextChargeOn
	}

	return s.storage.ResumeUser(ctx, params.UserID, *params.NextChargeOn, s.nowUTC())
}

func (s *Service) Disable(ctx context.Context, userID domain.UserID) (domain.User, error) {
	return s.storage.DisableUser(ctx, userID, s.nowUTC())
}

func (s *Service) nowUTC() time.Time {
	return s.now().UTC().Truncate(time.Second)
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
