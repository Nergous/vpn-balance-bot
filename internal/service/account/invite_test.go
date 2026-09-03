package account

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/Nergous/vpn-balance-bot/internal/domain"
)

func TestNewRejectsInvalidInviteTTL(t *testing.T) {
	service, err := New(&fakeStorage{}, 0)
	if !errors.Is(err, ErrInvalidInviteTTL) {
		t.Fatalf("New() error = %v, want %v", err, ErrInvalidInviteTTL)
	}
	if service != nil {
		t.Fatalf("New() service = %#v, want nil", service)
	}
}

func TestCreateInviteTokenStoresOnlyHash(t *testing.T) {
	now := time.Date(2026, time.August, 28, 10, 0, 0, 0, time.UTC)
	var stored CreateInviteTokenRecord
	storage := &fakeStorage{
		createInviteToken: func(_ context.Context, record CreateInviteTokenRecord) error {
			stored = record
			return nil
		},
	}
	service := newService(storage, 24*time.Hour, func() time.Time { return now })

	token, err := service.CreateInviteToken(context.Background(), CreateInviteTokenParams{AdminTelegramID: 1, UserID: 7})
	if err != nil {
		t.Fatalf("CreateInviteToken() error = %v", err)
	}
	if token == "" || strings.ContainsAny(token, "+/=") {
		t.Fatalf("token = %q, want non-empty URL-safe token", token)
	}
	if stored.TokenHash != hashInviteToken(token) {
		t.Fatal("storage did not receive token hash")
	}
	if strings.Contains(stored.TokenHash, token) {
		t.Fatal("storage record contains raw token")
	}
	if stored.UserID != 7 || !stored.CreatedAt.Equal(now) || !stored.ExpiresAt.Equal(now.Add(24*time.Hour)) {
		t.Fatalf("stored record = %#v", stored)
	}
}

func TestCreateInviteTokenRejectsMissingAdminActor(t *testing.T) {
	service := newService(&fakeStorage{}, time.Hour, time.Now)
	if _, err := service.CreateInviteToken(context.Background(), CreateInviteTokenParams{UserID: 7}); !errors.Is(err, ErrInvalidAdminTelegramID) {
		t.Fatalf("CreateInviteToken() error = %v, want %v", err, ErrInvalidAdminTelegramID)
	}
}

func TestConsumeInviteTokenHashesRawToken(t *testing.T) {
	now := time.Date(2026, time.August, 28, 10, 0, 0, 0, time.UTC)
	var stored ConsumeInviteTokenRecord
	storage := &fakeStorage{
		consumeInviteToken: func(_ context.Context, record ConsumeInviteTokenRecord) (domain.User, error) {
			stored = record
			return domain.User{ID: 1}, nil
		},
	}
	service := newService(storage, time.Hour, func() time.Time { return now })

	user, err := service.ConsumeInviteToken(context.Background(), ConsumeInviteParams{
		Token: "raw-token", TelegramUserID: 11, TelegramChatID: 22,
	})
	if err != nil {
		t.Fatalf("ConsumeInviteToken() error = %v", err)
	}
	if user.ID != 1 || stored.TokenHash != hashInviteToken("raw-token") ||
		stored.TelegramUserID != 11 || stored.TelegramChatID != 22 || !stored.ConsumedAt.Equal(now) {
		t.Fatalf("stored record = %#v", stored)
	}

	_, err = service.ConsumeInviteToken(context.Background(), ConsumeInviteParams{Token: " \t "})
	if !errors.Is(err, ErrInvalidInviteToken) {
		t.Fatalf("ConsumeInviteToken(blank) error = %v, want %v", err, ErrInvalidInviteToken)
	}
}
