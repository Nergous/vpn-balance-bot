package account

import (
	"context"
	cryptorand "crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"fmt"
	"strings"

	"github.com/Nergous/vpn-balance-bot/internal/domain"
)

const inviteTokenBytes = 32

// CreateInviteToken creates one raw, expiring token for a profile.
func (s *Service) CreateInviteToken(ctx context.Context, params CreateInviteTokenParams) (string, error) {
	if err := validateAdminTelegramID(params.AdminTelegramID); err != nil {
		return "", err
	}
	token, err := generateInviteToken()
	if err != nil {
		return "", err
	}

	createdAt := s.nowUTC()
	err = s.storage.CreateInviteToken(ctx, CreateInviteTokenRecord{
		TokenHash: hashInviteToken(token),
		UserID:    params.UserID,
		ExpiresAt: createdAt.Add(s.inviteTTL),
		CreatedAt: createdAt,
	})
	if err != nil {
		return "", err
	}

	return token, nil
}

// ConsumeInviteToken atomically binds a Telegram account to an invite profile.
func (s *Service) ConsumeInviteToken(ctx context.Context, params ConsumeInviteParams) (domain.User, error) {
	if strings.TrimSpace(params.Token) == "" {
		return domain.User{}, ErrInvalidInviteToken
	}

	return s.storage.ConsumeInviteToken(ctx, ConsumeInviteTokenRecord{
		TokenHash:      hashInviteToken(params.Token),
		TelegramUserID: params.TelegramUserID,
		TelegramChatID: params.TelegramChatID,
		ConsumedAt:     s.nowUTC(),
	})
}

func generateInviteToken() (string, error) {
	bytes := make([]byte, inviteTokenBytes)
	if _, err := cryptorand.Read(bytes); err != nil {
		return "", fmt.Errorf("%w: %w", ErrInviteTokenGenerate, err)
	}

	return base64.RawURLEncoding.EncodeToString(bytes), nil
}

func hashInviteToken(token string) string {
	sum := sha256.Sum256([]byte(token))
	return fmt.Sprintf("%x", sum)
}
