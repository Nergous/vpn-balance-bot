package sqlite

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/Nergous/vpn-balance-bot/internal/domain"
	"github.com/Nergous/vpn-balance-bot/internal/service/account"
)

func TestConsumeInviteTokenBindsUserOnce(t *testing.T) {
	ctx := context.Background()
	store := newInviteStore(t, ctx)
	now := time.Date(2026, time.August, 28, 10, 0, 0, 0, time.UTC)
	user := createInviteUser(t, store, ctx, 1, now)
	createInvite(t, store, ctx, "token-hash", user.ID, now, now.Add(time.Hour))

	bound, err := store.ConsumeInviteToken(ctx, account.ConsumeInviteTokenRecord{
		TokenHash: "token-hash", TelegramUserID: 101, TelegramChatID: 202, ConsumedAt: now,
	})
	if err != nil {
		t.Fatalf("ConsumeInviteToken() error = %v", err)
	}
	if bound.TelegramUserID == nil || *bound.TelegramUserID != 101 ||
		bound.TelegramChatID == nil || *bound.TelegramChatID != 202 {
		t.Fatalf("bound user = %#v", bound)
	}

	var usedAt int64
	if err := store.db.QueryRowContext(ctx, `SELECT used_at FROM invite_tokens WHERE token_hash = ?`, "token-hash").Scan(&usedAt); err != nil {
		t.Fatal(err)
	}
	if usedAt != now.Unix() {
		t.Fatalf("used_at = %d, want %d", usedAt, now.Unix())
	}

	_, err = store.ConsumeInviteToken(ctx, account.ConsumeInviteTokenRecord{
		TokenHash: "token-hash", TelegramUserID: 101, TelegramChatID: 202, ConsumedAt: now,
	})
	if !errors.Is(err, account.ErrInviteAlreadyUsed) {
		t.Fatalf("second ConsumeInviteToken() error = %v, want %v", err, account.ErrInviteAlreadyUsed)
	}
}

func TestConsumeInviteTokenRejectsExpiredAndUnknownToken(t *testing.T) {
	ctx := context.Background()
	store := newInviteStore(t, ctx)
	now := time.Date(2026, time.August, 28, 10, 0, 0, 0, time.UTC)
	user := createInviteUser(t, store, ctx, 1, now)
	createInvite(t, store, ctx, "expired-hash", user.ID, now.Add(-2*time.Hour), now.Add(-time.Hour))

	_, err := store.ConsumeInviteToken(ctx, account.ConsumeInviteTokenRecord{
		TokenHash: "expired-hash", TelegramUserID: 101, TelegramChatID: 202, ConsumedAt: now,
	})
	if !errors.Is(err, account.ErrInviteExpired) {
		t.Fatalf("ConsumeInviteToken(expired) error = %v, want %v", err, account.ErrInviteExpired)
	}

	_, err = store.ConsumeInviteToken(ctx, account.ConsumeInviteTokenRecord{
		TokenHash: "unknown-hash", TelegramUserID: 101, TelegramChatID: 202, ConsumedAt: now,
	})
	if !errors.Is(err, account.ErrInviteNotFound) {
		t.Fatalf("ConsumeInviteToken(unknown) error = %v, want %v", err, account.ErrInviteNotFound)
	}
}

func TestConsumeInviteTokenRollsBackOnTelegramConflict(t *testing.T) {
	ctx := context.Background()
	store := newInviteStore(t, ctx)
	now := time.Date(2026, time.August, 28, 10, 0, 0, 0, time.UTC)
	inviteUser := createInviteUser(t, store, ctx, 1, now)
	occupiedUser := createInviteUser(t, store, ctx, 2, now)
	telegramUserID := int64(101)
	occupiedUser.TelegramUserID = &telegramUserID
	if _, err := store.db.ExecContext(ctx, `UPDATE users SET telegram_user_id = ? WHERE id = ?`, telegramUserID, occupiedUser.ID); err != nil {
		t.Fatal(err)
	}
	createInvite(t, store, ctx, "conflict-hash", inviteUser.ID, now, now.Add(time.Hour))

	_, err := store.ConsumeInviteToken(ctx, account.ConsumeInviteTokenRecord{
		TokenHash: "conflict-hash", TelegramUserID: telegramUserID, TelegramChatID: 202, ConsumedAt: now,
	})
	if !errors.Is(err, account.ErrTelegramUserIDTaken) {
		t.Fatalf("ConsumeInviteToken() error = %v, want %v", err, account.ErrTelegramUserIDTaken)
	}

	var usedAt *int64
	if err := store.db.QueryRowContext(ctx, `SELECT used_at FROM invite_tokens WHERE token_hash = ?`, "conflict-hash").Scan(&usedAt); err != nil {
		t.Fatal(err)
	}
	if usedAt != nil {
		t.Fatalf("used_at = %d, want NULL", *usedAt)
	}

	userAfterFailure, err := store.UserByID(ctx, inviteUser.ID)
	if err != nil {
		t.Fatal(err)
	}
	if userAfterFailure.TelegramUserID != nil || userAfterFailure.TelegramChatID != nil {
		t.Fatalf("invite user was partially bound: %#v", userAfterFailure)
	}
}

func TestConsumeInviteTokenIsSingleUseUnderConcurrency(t *testing.T) {
	ctx := context.Background()
	store := newInviteStore(t, ctx)
	now := time.Date(2026, time.August, 28, 10, 0, 0, 0, time.UTC)
	user := createInviteUser(t, store, ctx, 1, now)
	createInvite(t, store, ctx, "concurrent-hash", user.ID, now, now.Add(time.Hour))

	start := make(chan struct{})
	errs := make(chan error, 2)
	var group sync.WaitGroup
	for range 2 {
		group.Add(1)
		go func() {
			defer group.Done()
			<-start
			_, err := store.ConsumeInviteToken(ctx, account.ConsumeInviteTokenRecord{
				TokenHash: "concurrent-hash", TelegramUserID: 101, TelegramChatID: 202, ConsumedAt: now,
			})
			errs <- err
		}()
	}
	close(start)
	group.Wait()
	close(errs)

	var successCount, alreadyUsedCount int
	for err := range errs {
		switch {
		case err == nil:
			successCount++
		case errors.Is(err, account.ErrInviteAlreadyUsed):
			alreadyUsedCount++
		default:
			t.Fatalf("concurrent ConsumeInviteToken() error = %v", err)
		}
	}
	if successCount != 1 || alreadyUsedCount != 1 {
		t.Fatalf("successes = %d, already used = %d", successCount, alreadyUsedCount)
	}
}

func TestCreateInviteTokenRejectsUnknownUser(t *testing.T) {
	store := newInviteStore(t, context.Background())
	err := store.CreateInviteToken(context.Background(), account.CreateInviteTokenRecord{
		TokenHash: "hash", UserID: 999, CreatedAt: time.Unix(1, 0), ExpiresAt: time.Unix(2, 0),
	})
	if !errors.Is(err, account.ErrNotFound) {
		t.Fatalf("CreateInviteToken() error = %v, want %v", err, account.ErrNotFound)
	}
}

func TestCreateInviteTokenRejectsAlreadyLinkedUser(t *testing.T) {
	ctx := context.Background()
	store := newInviteStore(t, ctx)
	now := time.Date(2026, time.September, 2, 10, 0, 0, 0, time.UTC)
	user := createInviteUser(t, store, ctx, 1, now)
	if _, err := store.db.ExecContext(ctx, `UPDATE users SET telegram_user_id = 101, telegram_chat_id = 202 WHERE id = ?`, user.ID); err != nil {
		t.Fatal(err)
	}
	err := store.CreateInviteToken(ctx, account.CreateInviteTokenRecord{TokenHash: "hash", UserID: user.ID, CreatedAt: now, ExpiresAt: now.Add(time.Hour)})
	if !errors.Is(err, account.ErrInviteUserLinked) {
		t.Fatalf("CreateInviteToken() error = %v, want %v", err, account.ErrInviteUserLinked)
	}
}

func TestCreateInviteTokenReplacesUnusedAndPreservesUsedAudit(t *testing.T) {
	ctx := context.Background()
	store := newInviteStore(t, ctx)
	now := time.Date(2026, time.September, 2, 10, 0, 0, 0, time.UTC)
	user := createInviteUser(t, store, ctx, 1, now)

	createInvite(t, store, ctx, "used-hash", user.ID, now, now.Add(time.Hour))
	usedAt := now.Add(time.Minute)
	if _, err := store.db.ExecContext(ctx, `UPDATE invite_tokens SET used_at = ? WHERE token_hash = ?`, usedAt.Unix(), "used-hash"); err != nil {
		t.Fatal(err)
	}
	createInvite(t, store, ctx, "old-unused-hash", user.ID, now.Add(2*time.Minute), now.Add(time.Hour))
	createInvite(t, store, ctx, "replacement-hash", user.ID, now.Add(3*time.Minute), now.Add(time.Hour))

	rows, err := store.db.QueryContext(ctx, `SELECT token_hash, used_at FROM invite_tokens WHERE user_id = ? ORDER BY token_hash`, user.ID)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()

	tokens := make(map[string]bool)
	for rows.Next() {
		var tokenHash string
		var tokenUsedAt *int64
		if err := rows.Scan(&tokenHash, &tokenUsedAt); err != nil {
			t.Fatal(err)
		}
		tokens[tokenHash] = tokenUsedAt != nil
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	if len(tokens) != 2 || !tokens["used-hash"] || tokens["replacement-hash"] || tokens["old-unused-hash"] {
		t.Fatalf("stored invite tokens = %#v", tokens)
	}
}

func TestCreateInviteTokenReissueIsAtomicUnderConcurrency(t *testing.T) {
	ctx := context.Background()
	store := newInviteStore(t, ctx)
	now := time.Date(2026, time.September, 2, 10, 0, 0, 0, time.UTC)
	user := createInviteUser(t, store, ctx, 1, now)
	createInvite(t, store, ctx, "initial-hash", user.ID, now, now.Add(time.Hour))

	start := make(chan struct{})
	errs := make(chan error, 2)
	var group sync.WaitGroup
	for _, tokenHash := range []string{"concurrent-a", "concurrent-b"} {
		tokenHash := tokenHash
		group.Add(1)
		go func() {
			defer group.Done()
			<-start
			errs <- store.CreateInviteToken(ctx, account.CreateInviteTokenRecord{
				TokenHash: tokenHash,
				UserID:    user.ID,
				CreatedAt: now.Add(time.Minute),
				ExpiresAt: now.Add(time.Hour),
			})
		}()
	}
	close(start)
	group.Wait()
	close(errs)

	for err := range errs {
		if err != nil {
			t.Fatalf("CreateInviteToken() error = %v", err)
		}
	}

	var count int
	var survivor string
	if err := store.db.QueryRowContext(ctx, `
		SELECT COUNT(*), COALESCE(MAX(token_hash), '')
		FROM invite_tokens
		WHERE user_id = ? AND used_at IS NULL
	`, user.ID).Scan(&count, &survivor); err != nil {
		t.Fatal(err)
	}
	if count != 1 || (survivor != "concurrent-a" && survivor != "concurrent-b") {
		t.Fatalf("unused count = %d, survivor = %q", count, survivor)
	}
}

func newInviteStore(t *testing.T, ctx context.Context) *Store {
	t.Helper()
	store := newTestSQLite(t, ctx)
	if err := store.Migrate(ctx); err != nil {
		t.Fatal(err)
	}
	return store
}

func createInviteUser(t *testing.T, store *Store, ctx context.Context, index int, now time.Time) domain.User {
	t.Helper()
	nextChargeOn, err := domain.NewDate(2026, time.September, index)
	if err != nil {
		t.Fatal(err)
	}
	user, err := store.CreateUser(ctx, domain.User{
		DisplayName: "Invite user", MonthlyFeeMinor: 100000, Currency: "RUB",
		BillingAnchorDay: index, NextChargeOn: nextChargeOn,
		Status: domain.UserStatusActive, CreatedAt: now, UpdatedAt: now,
	})
	if err != nil {
		t.Fatal(err)
	}
	return user
}

func createInvite(t *testing.T, store *Store, ctx context.Context, hash string, userID domain.UserID, createdAt, expiresAt time.Time) {
	t.Helper()
	if err := store.CreateInviteToken(ctx, account.CreateInviteTokenRecord{
		TokenHash: hash, UserID: userID, CreatedAt: createdAt, ExpiresAt: expiresAt,
	}); err != nil {
		t.Fatal(err)
	}
}
