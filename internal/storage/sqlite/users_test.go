package sqlite_test

import (
	"context"
	"errors"
	"reflect"
	"sync"
	"testing"
	"time"

	"github.com/Nergous/vpn-balance-bot/internal/domain"
	"github.com/Nergous/vpn-balance-bot/internal/service/account"
	storageSQLite "github.com/Nergous/vpn-balance-bot/internal/storage/sqlite"
	"github.com/Nergous/vpn-balance-bot/internal/testutil"
)

var _ account.Storage = (*storageSQLite.Store)(nil)

func TestCreateAndReadUser(t *testing.T) {
	store := testutil.NewSQLite(t)
	ctx := context.Background()
	want := newUser(t, 1, domain.UserStatusActive)

	created, err := store.CreateUser(ctx, want)
	if err != nil {
		t.Fatalf("CreateUser() error = %v", err)
	}
	if created.ID <= 0 {
		t.Fatalf("CreateUser() ID = %d, want positive", created.ID)
	}

	want.ID = created.ID
	want.CreatedAt = want.CreatedAt.UTC().Truncate(time.Second)
	want.UpdatedAt = want.UpdatedAt.UTC().Truncate(time.Second)
	if !reflect.DeepEqual(created, want) {
		t.Fatalf("CreateUser() = %#v, want %#v", created, want)
	}

	byID, err := store.UserByID(ctx, created.ID)
	if err != nil {
		t.Fatalf("UserByID() error = %v", err)
	}
	if !reflect.DeepEqual(byID, want) {
		t.Fatalf("UserByID() = %#v, want %#v", byID, want)
	}

	byTelegramID, err := store.UserByTelegramID(ctx, *want.TelegramUserID)
	if err != nil {
		t.Fatalf("UserByTelegramID() error = %v", err)
	}
	if !reflect.DeepEqual(byTelegramID, want) {
		t.Fatalf("UserByTelegramID() = %#v, want %#v", byTelegramID, want)
	}
}

func TestCreateUserAllowsNullableTelegramFields(t *testing.T) {
	store := testutil.NewSQLite(t)
	user := newUser(t, 1, domain.UserStatusActive)
	user.TelegramUserID = nil
	user.TelegramChatID = nil
	user.Username = nil

	created, err := store.CreateUser(context.Background(), user)
	if err != nil {
		t.Fatalf("CreateUser() error = %v", err)
	}
	if created.TelegramUserID != nil || created.TelegramChatID != nil || created.Username != nil {
		t.Fatalf("CreateUser() nullable fields = %#v, %#v, %#v", created.TelegramUserID, created.TelegramChatID, created.Username)
	}

	read, err := store.UserByID(context.Background(), created.ID)
	if err != nil {
		t.Fatalf("UserByID() error = %v", err)
	}
	if read.TelegramUserID != nil || read.TelegramChatID != nil || read.Username != nil {
		t.Fatalf("UserByID() nullable fields = %#v, %#v, %#v", read.TelegramUserID, read.TelegramChatID, read.Username)
	}
}

func TestCreateUserMapsTelegramConflicts(t *testing.T) {
	store := testutil.NewSQLite(t)
	ctx := context.Background()
	first := newUser(t, 1, domain.UserStatusActive)
	if _, err := store.CreateUser(ctx, first); err != nil {
		t.Fatalf("CreateUser(first) error = %v", err)
	}

	duplicateUserID := newUser(t, 2, domain.UserStatusActive)
	duplicateUserID.TelegramUserID = first.TelegramUserID
	if _, err := store.CreateUser(ctx, duplicateUserID); !errors.Is(err, account.ErrTelegramUserIDTaken) {
		t.Fatalf("CreateUser(duplicate user ID) error = %v, want %v", err, account.ErrTelegramUserIDTaken)
	}

	duplicateChatID := newUser(t, 3, domain.UserStatusActive)
	duplicateChatID.TelegramChatID = first.TelegramChatID
	if _, err := store.CreateUser(ctx, duplicateChatID); !errors.Is(err, account.ErrTelegramChatIDTaken) {
		t.Fatalf("CreateUser(duplicate chat ID) error = %v, want %v", err, account.ErrTelegramChatIDTaken)
	}
}

func TestListUsersWithOptionalStatus(t *testing.T) {
	store := testutil.NewSQLite(t)
	ctx := context.Background()

	for index, status := range []domain.UserStatus{
		domain.UserStatusActive,
		domain.UserStatusPaused,
		domain.UserStatusDisabled,
	} {
		if _, err := store.CreateUser(ctx, newUser(t, index+1, status)); err != nil {
			t.Fatalf("CreateUser(%s) error = %v", status, err)
		}
	}

	all, err := store.ListUsers(ctx, nil)
	if err != nil {
		t.Fatalf("ListUsers(nil) error = %v", err)
	}
	if len(all) != 3 {
		t.Fatalf("ListUsers(nil) count = %d, want 3", len(all))
	}
	for index, user := range all {
		wantID := domain.UserID(index + 1)
		if user.ID != wantID {
			t.Fatalf("ListUsers(nil)[%d].ID = %d, want %d", index, user.ID, wantID)
		}
	}

	status := domain.UserStatusPaused
	paused, err := store.ListUsers(ctx, &status)
	if err != nil {
		t.Fatalf("ListUsers(paused) error = %v", err)
	}
	if len(paused) != 1 || paused[0].Status != domain.UserStatusPaused {
		t.Fatalf("ListUsers(paused) = %#v", paused)
	}
}

func TestListUsersPageAndStatusCounts(t *testing.T) {
	store := testutil.NewSQLite(t)
	ctx := context.Background()
	now := time.Date(2026, time.September, 3, 10, 0, 0, 0, time.UTC)
	statuses := []domain.UserStatus{
		domain.UserStatusActive,
		domain.UserStatusPaused,
		domain.UserStatusDisabled,
		domain.UserStatusActive,
		domain.UserStatusPaused,
	}
	users := make([]domain.User, 0, len(statuses))
	for index, status := range statuses {
		user := newUser(t, index+1, status)
		if index == len(statuses)-1 {
			user.TelegramUserID = nil
			user.TelegramChatID = nil
		}
		created, err := store.CreateUser(ctx, user)
		if err != nil {
			t.Fatal(err)
		}
		users = append(users, created)
	}
	for _, entry := range []domain.LedgerEntry{
		{UserID: users[0].ID, Kind: domain.LedgerKindAdjustment, AmountMinor: -100, OccurredAt: now, CreatedAt: now},
		{UserID: users[3].ID, Kind: domain.LedgerKindPayment, AmountMinor: 200000, OccurredAt: now, CreatedAt: now},
	} {
		if _, err := store.CreateLedgerEntry(ctx, entry); err != nil {
			t.Fatal(err)
		}
	}
	billingDate, err := domain.NewDate(2026, time.September, 3)
	if err != nil {
		t.Fatal(err)
	}
	delivery := domain.ReminderDelivery{
		UserID: users[0].ID, BillingDate: billingDate, ScheduledDate: billingDate,
		ReminderType: domain.ReminderTypeManual, Status: domain.ReminderStatusPending,
		CreatedAt: now, UpdatedAt: now,
	}
	if _, created, err := store.CreateReminderDelivery(ctx, delivery); err != nil || !created {
		t.Fatalf("CreateReminderDelivery() = %t, %v", created, err)
	}
	unreachable := "unreachable"
	delivery.Status = domain.ReminderStatusFailed
	delivery.ErrorCode = &unreachable
	delivery.UpdatedAt = now.Add(time.Second)
	if err := store.UpdateReminderDelivery(ctx, delivery); err != nil {
		t.Fatal(err)
	}

	page, hasMore, err := store.ListUsersPage(ctx, nil, 1, 2)
	if err != nil {
		t.Fatal(err)
	}
	if len(page) != 2 || page[0].ID != 2 || page[1].ID != 3 || !hasMore {
		t.Fatalf("page=%#v hasMore=%v", page, hasMore)
	}
	active := domain.UserStatusActive
	page, hasMore, err = store.ListUsersPage(ctx, &active, 1, 2)
	if err != nil {
		t.Fatal(err)
	}
	if len(page) != 1 || page[0].ID != 4 || hasMore {
		t.Fatalf("active page=%#v hasMore=%v", page, hasMore)
	}

	counts, err := store.UserStatusCounts(ctx)
	if err != nil {
		t.Fatal(err)
	}
	want := account.UserStatusCounts{
		Total: 5, Active: 2, Paused: 2, Disabled: 1,
		Debtors: 1, Insufficient: 1, Unlinked: 1, Unreachable: 1,
	}
	if counts != want {
		t.Fatalf("counts=%#v want=%#v", counts, want)
	}
}

func TestUserMutations(t *testing.T) {
	store := testutil.NewSQLite(t)
	ctx := context.Background()
	created, err := store.CreateUser(ctx, newUser(t, 1, domain.UserStatusActive))
	if err != nil {
		t.Fatalf("CreateUser() error = %v", err)
	}

	feeUpdatedAt := time.Date(2026, time.September, 1, 12, 0, 0, 0, time.UTC)
	updated, err := store.SetMonthlyFee(ctx, created.ID, 250000, feeUpdatedAt)
	if err != nil {
		t.Fatalf("SetMonthlyFee() error = %v", err)
	}
	if updated.MonthlyFeeMinor != 250000 || !updated.UpdatedAt.Equal(feeUpdatedAt) {
		t.Fatalf("SetMonthlyFee() = %#v", updated)
	}
	if updated.NextChargeOn != created.NextChargeOn {
		t.Fatalf("SetMonthlyFee() next charge = %s, want %s", updated.NextChargeOn, created.NextChargeOn)
	}

	pausedAt := feeUpdatedAt.Add(time.Hour)
	paused, err := store.PauseUser(ctx, created.ID, pausedAt)
	if err != nil {
		t.Fatalf("PauseUser() error = %v", err)
	}
	if paused.Status != domain.UserStatusPaused || paused.NextChargeOn != created.NextChargeOn || !paused.UpdatedAt.Equal(pausedAt) {
		t.Fatalf("PauseUser() = %#v", paused)
	}

	nextChargeOn, err := domain.NewDate(2026, time.October, 17)
	if err != nil {
		t.Fatal(err)
	}
	resumedAt := pausedAt.Add(time.Hour)
	resumed, err := store.ResumeUser(ctx, created.ID, nextChargeOn, resumedAt)
	if err != nil {
		t.Fatalf("ResumeUser() error = %v", err)
	}
	if resumed.Status != domain.UserStatusActive || resumed.NextChargeOn != nextChargeOn || !resumed.UpdatedAt.Equal(resumedAt) {
		t.Fatalf("ResumeUser() = %#v", resumed)
	}

	disabledAt := resumedAt.Add(time.Hour)
	disabled, err := store.DisableUser(ctx, created.ID, disabledAt)
	if err != nil {
		t.Fatalf("DisableUser() error = %v", err)
	}
	if disabled.Status != domain.UserStatusDisabled || disabled.NextChargeOn != nextChargeOn || !disabled.UpdatedAt.Equal(disabledAt) {
		t.Fatalf("DisableUser() = %#v", disabled)
	}
}

func TestUserLifecycleRejectsInvalidTransitions(t *testing.T) {
	store := testutil.NewSQLite(t)
	ctx := context.Background()
	active, err := store.CreateUser(ctx, newUser(t, 1, domain.UserStatusActive))
	if err != nil {
		t.Fatal(err)
	}
	paused, err := store.CreateUser(ctx, newUser(t, 2, domain.UserStatusPaused))
	if err != nil {
		t.Fatal(err)
	}
	disabled, err := store.CreateUser(ctx, newUser(t, 3, domain.UserStatusDisabled))
	if err != nil {
		t.Fatal(err)
	}
	nextChargeOn, err := domain.NewDate(2026, time.October, 17)
	if err != nil {
		t.Fatal(err)
	}
	updatedAt := time.Date(2026, time.September, 2, 12, 0, 0, 0, time.UTC)

	tests := []struct {
		name string
		call func() error
	}{
		{name: "pause paused", call: func() error { _, err := store.PauseUser(ctx, paused.ID, updatedAt); return err }},
		{name: "resume active", call: func() error { _, err := store.ResumeUser(ctx, active.ID, nextChargeOn, updatedAt); return err }},
		{name: "pause disabled", call: func() error { _, err := store.PauseUser(ctx, disabled.ID, updatedAt); return err }},
		{name: "resume disabled", call: func() error { _, err := store.ResumeUser(ctx, disabled.ID, nextChargeOn, updatedAt); return err }},
		{name: "disable disabled", call: func() error { _, err := store.DisableUser(ctx, disabled.ID, updatedAt); return err }},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			err := test.call()
			if !errors.Is(err, account.ErrInvalidUserStatusTransition) {
				t.Fatalf("error = %v, want %v", err, account.ErrInvalidUserStatusTransition)
			}
			if errors.Is(err, account.ErrNotFound) {
				t.Fatalf("error = %v, must not report not found", err)
			}
		})
	}
}

func TestPauseUserTransitionIsAtomicUnderConcurrency(t *testing.T) {
	store := testutil.NewSQLite(t)
	ctx := context.Background()
	user, err := store.CreateUser(ctx, newUser(t, 1, domain.UserStatusActive))
	if err != nil {
		t.Fatal(err)
	}
	updatedAt := time.Date(2026, time.September, 2, 12, 0, 0, 0, time.UTC)

	start := make(chan struct{})
	errs := make(chan error, 2)
	var group sync.WaitGroup
	for range 2 {
		group.Add(1)
		go func() {
			defer group.Done()
			<-start
			_, err := store.PauseUser(ctx, user.ID, updatedAt)
			errs <- err
		}()
	}
	close(start)
	group.Wait()
	close(errs)

	var successCount, invalidTransitionCount int
	for err := range errs {
		switch {
		case err == nil:
			successCount++
		case errors.Is(err, account.ErrInvalidUserStatusTransition):
			invalidTransitionCount++
		default:
			t.Fatalf("PauseUser() error = %v", err)
		}
	}
	if successCount != 1 || invalidTransitionCount != 1 {
		t.Fatalf("successes = %d, invalid transitions = %d", successCount, invalidTransitionCount)
	}
}

func TestUserMethodsReturnNotFound(t *testing.T) {
	store := testutil.NewSQLite(t)
	ctx := context.Background()
	updatedAt := time.Date(2026, time.September, 1, 12, 0, 0, 0, time.UTC)
	nextChargeOn, err := domain.NewDate(2026, time.October, 17)
	if err != nil {
		t.Fatal(err)
	}

	checks := []struct {
		name string
		call func() error
	}{
		{name: "by ID", call: func() error { _, err := store.UserByID(ctx, 999); return err }},
		{name: "by Telegram ID", call: func() error { _, err := store.UserByTelegramID(ctx, 999); return err }},
		{name: "set fee", call: func() error { _, err := store.SetMonthlyFee(ctx, 999, 1000, updatedAt); return err }},
		{name: "pause", call: func() error { _, err := store.PauseUser(ctx, 999, updatedAt); return err }},
		{name: "resume", call: func() error { _, err := store.ResumeUser(ctx, 999, nextChargeOn, updatedAt); return err }},
		{name: "disable", call: func() error { _, err := store.DisableUser(ctx, 999, updatedAt); return err }},
	}

	for _, check := range checks {
		t.Run(check.name, func(t *testing.T) {
			if err := check.call(); !errors.Is(err, account.ErrNotFound) {
				t.Fatalf("error = %v, want %v", err, account.ErrNotFound)
			}
		})
	}
}

func TestResumeUserRejectsInvalidDate(t *testing.T) {
	store := testutil.NewSQLite(t)
	_, err := store.ResumeUser(
		context.Background(),
		1,
		domain.Date{Year: 2026, Month: time.February, Day: 30},
		time.Now(),
	)
	if !errors.Is(err, domain.ErrInvalidDate) {
		t.Fatalf("ResumeUser() error = %v, want %v", err, domain.ErrInvalidDate)
	}
}

func TestCreateUserRespectsCanceledContext(t *testing.T) {
	store := testutil.NewSQLite(t)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := store.CreateUser(ctx, newUser(t, 1, domain.UserStatusActive))
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("CreateUser() error = %v, want %v", err, context.Canceled)
	}
}

func newUser(t *testing.T, index int, status domain.UserStatus) domain.User {
	t.Helper()

	nextChargeOn, err := domain.NewDate(2026, time.September, index)
	if err != nil {
		t.Fatal(err)
	}

	telegramUserID := int64(1000 + index)
	telegramChatID := int64(2000 + index)
	username := "user" + string(rune('a'+index-1))
	createdAt := time.Date(2026, time.August, 28, 10, 0, index, 123000000, time.FixedZone("test", 3*60*60))

	return domain.User{
		TelegramUserID:   &telegramUserID,
		TelegramChatID:   &telegramChatID,
		Username:         &username,
		DisplayName:      "User " + username,
		MonthlyFeeMinor:  domain.AmountMinor(100000 + index),
		Currency:         "RUB",
		BillingAnchorDay: index,
		NextChargeOn:     nextChargeOn,
		Status:           status,
		CreatedAt:        createdAt,
		UpdatedAt:        createdAt,
	}
}
