package telegram

import (
	"context"
	"errors"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/Nergous/vpn-balance-bot/internal/domain"
	"github.com/Nergous/vpn-balance-bot/internal/service/account"
	"github.com/Nergous/vpn-balance-bot/internal/testutil"
)

func TestAdminRejectsNonAdminBeforeMutation(t *testing.T) {
	client := &testutil.FakeTelegramClient{}
	service := &fakeAdmin{}
	admin := NewAdmin(client, service, &fakeAdminReminders{}, 1, LanguageRussian)
	if err := admin.Pause(context.Background(), IncomingMessage{ChatID: 2, UserID: 2, ChatType: ChatTypePrivate}, 7); err != nil {
		t.Fatal(err)
	}
	if service.pauseCalls != 0 {
		t.Fatal("non-admin reached service")
	}
}
func TestPaymentWizardConfirmsOnce(t *testing.T) {
	service := &fakeAdmin{}
	wizard := NewWizard()
	if _, err := wizard.BeginPayment(context.Background(), 1, PaymentDraft{UserID: 7, AmountMinor: 100}); err != nil {
		t.Fatal(err)
	}
	if err := wizard.ConfirmPayment(context.Background(), 1, service); err != nil {
		t.Fatal(err)
	}
	if service.paymentCalls != 1 {
		t.Fatalf("payment calls=%d", service.paymentCalls)
	}
	if err := wizard.ConfirmPayment(context.Background(), 1, service); !errors.Is(err, ErrWizardNotFound) {
		t.Fatalf("second confirmation=%v", err)
	}
}

func TestPaymentWizardKeepsDraftWhenLedgerWriteFails(t *testing.T) {
	wizard := NewWizard()
	ctx := context.Background()
	if _, err := wizard.BeginPayment(ctx, 1, PaymentDraft{UserID: 7, AmountMinor: 100}); err != nil {
		t.Fatal(err)
	}
	service := &fakeAdmin{paymentErr: errors.New("database unavailable")}
	if err := wizard.ConfirmPayment(ctx, 1, service); err == nil {
		t.Fatal("ConfirmPayment() unexpectedly succeeded")
	}
	service.paymentErr = nil
	if err := wizard.ConfirmPayment(ctx, 1, service); err != nil {
		t.Fatalf("retry ConfirmPayment() error = %v", err)
	}
	if service.paymentCalls != 2 {
		t.Fatalf("payment calls = %d, want 2", service.paymentCalls)
	}
}
func TestAdminDashboardAndCreateUser(t *testing.T) {
	client := &testutil.FakeTelegramClient{}
	service := &fakeAdmin{}
	admin := NewAdmin(client, service, &fakeAdminReminders{}, 1, LanguageRussian)
	if err := admin.Dashboard(context.Background(), IncomingMessage{ChatID: 1, UserID: 1, ChatType: ChatTypePrivate}); err != nil {
		t.Fatal(err)
	}
	if _, err := admin.CreateUser(context.Background(), IncomingMessage{UserID: 2, ChatType: ChatTypePrivate}, account.CreateUserParams{}); !errors.Is(err, ErrAdminOnly) {
		t.Fatalf("CreateUser error=%v", err)
	}
}

func TestAdminPassesActorIDToProfileServiceBoundary(t *testing.T) {
	client := &testutil.FakeTelegramClient{}
	service := &fakeAdmin{}
	admin := NewAdmin(client, service, &fakeAdminReminders{}, 17, LanguageEnglish)
	message := IncomingMessage{ChatID: 17, UserID: 17, ChatType: ChatTypePrivate}
	date, _ := domain.NewDate(2026, time.September, 3)

	if _, err := admin.CreateUser(context.Background(), message, account.CreateUserParams{}); err != nil {
		t.Fatal(err)
	}
	if err := admin.CreateInvite(context.Background(), message, 7); err != nil {
		t.Fatal(err)
	}
	if err := admin.ChangeFee(context.Background(), message, account.ChangeMonthlyFeeParams{}); err != nil {
		t.Fatal(err)
	}
	if err := admin.Pause(context.Background(), message, 7); err != nil {
		t.Fatal(err)
	}
	if err := admin.Resume(context.Background(), message, account.ResumeParams{NextChargeOn: &date}); err != nil {
		t.Fatal(err)
	}
	if err := admin.Disable(context.Background(), message, 7); err != nil {
		t.Fatal(err)
	}
	if len(service.actorIDs) != 6 {
		t.Fatalf("actor IDs = %#v", service.actorIDs)
	}
	for _, actorID := range service.actorIDs {
		if actorID != 17 {
			t.Fatalf("actor IDs = %#v", service.actorIDs)
		}
	}
}

func TestAdminUsesBoundedQueryCapabilities(t *testing.T) {
	client := &testutil.FakeTelegramClient{}
	service := &fakeAdminQueries{
		fakeAdmin: &fakeAdmin{},
		counts:    account.UserStatusCounts{Total: 4, Active: 2, Paused: 1, Disabled: 1},
		page:      []domain.User{{ID: 8, DisplayName: "Alice", Status: domain.UserStatusActive}, {ID: 9, DisplayName: "Bob", Status: domain.UserStatusPaused}},
		hasMore:   true,
	}
	admin := NewAdmin(client, service, &fakeAdminReminders{}, 1, LanguageEnglish)
	message := IncomingMessage{ChatID: 1, UserID: 1, ChatType: ChatTypePrivate}

	if err := admin.Dashboard(context.Background(), message); err != nil {
		t.Fatal(err)
	}
	status := domain.UserStatusActive
	if err := admin.Users(context.Background(), message, account.UserFilter{Status: &status}, 7); err != nil {
		t.Fatal(err)
	}
	if service.countCalls != 1 || service.pageCalls != 1 || service.afterID != 7 || service.limit != adminUserPageSize {
		t.Fatalf("counts=%d pages=%d after=%d limit=%d", service.countCalls, service.pageCalls, service.afterID, service.limit)
	}
	if len(client.Sent) != 2 || !strings.Contains(client.Sent[0].Text, "Users: 4") || !strings.Contains(client.Sent[1].Text, "/admin users active 9") {
		t.Fatalf("sent=%#v", client.Sent)
	}
}

func TestAdminCreateInviteSendsCopyableSpoilerToken(t *testing.T) {
	client := &testutil.FakeTelegramClient{}
	admin := NewAdmin(client, &fakeAdmin{}, &fakeAdminReminders{}, 1, LanguageEnglish)
	if err := admin.CreateInvite(context.Background(), IncomingMessage{ChatID: 1, UserID: 1, ChatType: ChatTypePrivate}, 7); err != nil {
		t.Fatal(err)
	}
	if len(client.Sent) != 1 || client.Sent[0].Text != "Invite token:" || client.Sent[0].InviteToken != "token" || client.Sent[0].CopyLabel != "Copy token" {
		t.Fatalf("sent = %#v", client.Sent)
	}
}

func TestAdminCommandRouterAuthorizesAndConfirmsPaymentOnce(t *testing.T) {
	client := &testutil.FakeTelegramClient{}
	service := &fakeAdmin{}
	bot := NewWithClient(client, service, LanguageRussian)
	if err := bot.EnableAdmin(1, &fakeAdminReminders{}); err != nil {
		t.Fatal(err)
	}
	if err := bot.HandleAdminCommand(context.Background(), IncomingMessage{ChatID: 2, UserID: 2, ChatType: ChatTypePrivate, Text: "/admin payment 7 100"}); err != nil {
		t.Fatal(err)
	}
	if service.paymentCalls != 0 {
		t.Fatal("non-admin command reached payment service")
	}
	if err := bot.HandleAdminCommand(context.Background(), IncomingMessage{ChatID: 1, UserID: 1, ChatType: ChatTypePrivate, Text: "/admin payment 7 100 note"}); err != nil {
		t.Fatal(err)
	}
	if err := bot.HandleAdminCommand(context.Background(), IncomingMessage{ChatID: 1, UserID: 1, ChatType: ChatTypePrivate, Text: "/admin confirm"}); err != nil {
		t.Fatal(err)
	}
	if service.paymentCalls != 1 {
		t.Fatalf("payment calls = %d", service.paymentCalls)
	}
	if err := bot.HandleAdminCommand(context.Background(), IncomingMessage{ChatID: 1, UserID: 1, ChatType: ChatTypePrivate, Text: "/admin confirm"}); err != nil {
		t.Fatalf("second confirmation = %v", err)
	}
	if len(client.Sent) < 4 || !strings.Contains(client.Sent[len(client.Sent)-1].Text, "ожидающего подтверждения") {
		t.Fatalf("sent = %#v", client.Sent)
	}
}

func TestAdminPaymentConfirmationIncludesAuditDetailsAndPreservesDate(t *testing.T) {
	client := &testutil.FakeTelegramClient{}
	service := &fakeAdmin{}
	admin := NewAdmin(client, service, &fakeAdminReminders{}, 1, LanguageEnglish)
	now := time.Date(2026, time.September, 3, 9, 15, 27, 0, time.UTC)
	admin.now = func() time.Time { return now }
	note := "September subscription"
	message := IncomingMessage{ChatID: 1, UserID: 1, ChatType: ChatTypePrivate}

	if err := admin.BeginPayment(context.Background(), message, PaymentDraft{UserID: 7, AmountMinor: 25000, Note: &note}); err != nil {
		t.Fatal(err)
	}
	if len(client.Sent) != 1 {
		t.Fatalf("sent = %#v", client.Sent)
	}
	for _, expected := range []string{"#7 User 7", "250.00 RUB", "2026-09-03 09:15 UTC", note, "/admin confirm", "/admin cancel"} {
		if !strings.Contains(client.Sent[0].Text, expected) {
			t.Fatalf("confirmation missing %q: %q", expected, client.Sent[0].Text)
		}
	}
	if err := admin.ConfirmPayment(context.Background(), message); err != nil {
		t.Fatal(err)
	}
	if service.paymentParams.OccurredAt == nil || !service.paymentParams.OccurredAt.Equal(now) {
		t.Fatalf("payment occurred at = %#v", service.paymentParams.OccurredAt)
	}
	if len(client.Sent) != 2 || client.Sent[1].Text != "Payment recorded." {
		t.Fatalf("sent = %#v", client.Sent)
	}
}

func TestAdminExpectedErrorsAreAcknowledgedButInfrastructureErrorsRetry(t *testing.T) {
	message := IncomingMessage{ChatID: 1, UserID: 1, ChatType: ChatTypePrivate, Text: "/admin pause 7"}
	client := &testutil.FakeTelegramClient{}
	service := &fakeAdmin{pauseErr: account.ErrInvalidUserStatusTransition}
	bot := NewWithClient(client, service, LanguageEnglish)
	if err := bot.EnableAdmin(1, &fakeAdminReminders{}); err != nil {
		t.Fatal(err)
	}
	if err := bot.HandleAdminCommand(context.Background(), message); err != nil {
		t.Fatalf("expected business error returned: %v", err)
	}
	if len(client.Sent) != 1 || client.Sent[0].Text != "The action is not allowed in the current state." {
		t.Fatalf("sent = %#v", client.Sent)
	}

	sentinel := errors.New("database unavailable")
	client = &testutil.FakeTelegramClient{}
	service = &fakeAdmin{pauseErr: sentinel}
	bot = NewWithClient(client, service, LanguageEnglish)
	if err := bot.EnableAdmin(1, &fakeAdminReminders{}); err != nil {
		t.Fatal(err)
	}
	if err := bot.HandleAdminCommand(context.Background(), message); !errors.Is(err, sentinel) {
		t.Fatalf("infrastructure error = %v", err)
	}
	if len(client.Sent) != 0 {
		t.Fatalf("unexpected response = %#v", client.Sent)
	}
}

func TestAdminManualReminderRequiresAdminAndUsesReminderService(t *testing.T) {
	client := &testutil.FakeTelegramClient{}
	service := &fakeAdmin{}
	reminders := &fakeAdminReminders{}
	admin := NewAdmin(client, service, reminders, 1, LanguageRussian)
	if err := admin.RemindNow(context.Background(), IncomingMessage{ChatID: 2, UserID: 2, ChatType: ChatTypePrivate}, 7, "top up"); err != nil {
		t.Fatal(err)
	}
	if reminders.calls != 0 {
		t.Fatal("non-admin reached reminder service")
	}
	if err := admin.RemindNow(context.Background(), IncomingMessage{ChatID: 1, UserID: 1, ChatType: ChatTypePrivate}, 7, "top up"); err != nil {
		t.Fatal(err)
	}
	if reminders.calls != 1 || reminders.user.ID != 7 || reminders.text != "top up" {
		t.Fatalf("reminder = %#v", reminders)
	}
}

func TestAdminReconcileRequiresAndForwardsExactManualDeliveryKey(t *testing.T) {
	client := &testutil.FakeTelegramClient{}
	reminders := &fakeAdminReminders{}
	bot := NewWithClient(client, &fakeAdmin{}, LanguageEnglish)
	if err := bot.EnableAdmin(1, reminders); err != nil {
		t.Fatal(err)
	}
	message := IncomingMessage{ChatID: 1, UserID: 1, ChatType: ChatTypePrivate}
	message.Text = "/admin reconcile 7 2026-09-03 manual"
	if err := bot.HandleAdminCommand(context.Background(), message); err != nil {
		t.Fatal(err)
	}
	if reminders.reconcileCalls != 0 {
		t.Fatal("manual reconciliation without delivery key reached service")
	}
	message.Text = "/admin reconcile 7 2026-09-03 manual update:42"
	if err := bot.HandleAdminCommand(context.Background(), message); err != nil {
		t.Fatal(err)
	}
	if reminders.reconcileCalls != 1 || reminders.reconcileKey != "update:42" {
		t.Fatalf("reconciliation calls=%d key=%q", reminders.reconcileCalls, reminders.reconcileKey)
	}
}

type fakeAdmin struct {
	pauseCalls, paymentCalls int
	paymentErr               error
	pauseErr                 error
	paymentParams            account.AddPaymentParams
	actorIDs                 []int64
}

type fakeAdminQueries struct {
	*fakeAdmin
	counts     account.UserStatusCounts
	page       []domain.User
	hasMore    bool
	countCalls int
	pageCalls  int
	afterID    domain.UserID
	limit      int
}

func (f *fakeAdminQueries) UserStatusCounts(context.Context) (account.UserStatusCounts, error) {
	f.countCalls++
	return f.counts, nil
}

func (f *fakeAdminQueries) ListUsersPage(_ context.Context, _ account.UserFilter, afterID domain.UserID, limit int) ([]domain.User, bool, error) {
	f.pageCalls++
	f.afterID = afterID
	f.limit = limit
	return f.page, f.hasMore, nil
}

func (f *fakeAdmin) ConsumeInviteToken(context.Context, account.ConsumeInviteParams) (domain.User, error) {
	return domain.User{}, nil
}
func (f *fakeAdmin) UserByTelegramID(context.Context, int64) (domain.User, error) {
	return domain.User{}, nil
}
func (f *fakeAdmin) Balance(context.Context, domain.UserID) (domain.AmountMinor, error) {
	return 0, nil
}
func (f *fakeAdmin) LastLedgerEntries(context.Context, domain.UserID) ([]domain.LedgerEntry, error) {
	return nil, nil
}
func (f *fakeAdmin) CreateUser(_ context.Context, params account.CreateUserParams) (domain.User, error) {
	f.actorIDs = append(f.actorIDs, params.AdminTelegramID)
	return domain.User{ID: 8, DisplayName: "Created"}, nil
}
func (f *fakeAdmin) UserByID(_ context.Context, userID domain.UserID) (domain.User, error) {
	return domain.User{ID: userID, DisplayName: "User " + strconv.FormatInt(int64(userID), 10), Currency: "RUB"}, nil
}
func (f *fakeAdmin) ListUsers(context.Context, account.UserFilter) ([]domain.User, error) {
	return nil, nil
}
func (f *fakeAdmin) CreateInviteToken(_ context.Context, params account.CreateInviteTokenParams) (string, error) {
	f.actorIDs = append(f.actorIDs, params.AdminTelegramID)
	return "token", nil
}
func (f *fakeAdmin) ChangeMonthlyFee(_ context.Context, params account.ChangeMonthlyFeeParams) (domain.User, error) {
	f.actorIDs = append(f.actorIDs, params.AdminTelegramID)
	return domain.User{}, nil
}
func (f *fakeAdmin) Pause(_ context.Context, params account.AdminUserParams) (domain.User, error) {
	f.actorIDs = append(f.actorIDs, params.AdminTelegramID)
	f.pauseCalls++
	return domain.User{}, f.pauseErr
}
func (f *fakeAdmin) Resume(_ context.Context, params account.ResumeParams) (domain.User, error) {
	f.actorIDs = append(f.actorIDs, params.AdminTelegramID)
	return domain.User{}, nil
}
func (f *fakeAdmin) Disable(_ context.Context, params account.AdminUserParams) (domain.User, error) {
	f.actorIDs = append(f.actorIDs, params.AdminTelegramID)
	return domain.User{}, nil
}
func (f *fakeAdmin) AddPayment(_ context.Context, params account.AddPaymentParams) (domain.LedgerEntry, error) {
	f.paymentCalls++
	f.paymentParams = params
	return domain.LedgerEntry{}, f.paymentErr
}
func (f *fakeAdmin) AddOpeningBalance(context.Context, account.AddOpeningBalanceParams) (domain.LedgerEntry, error) {
	return domain.LedgerEntry{}, nil
}
func (f *fakeAdmin) AddAdjustment(context.Context, account.AddAdjustmentParams) (domain.LedgerEntry, error) {
	return domain.LedgerEntry{}, nil
}
func (f *fakeAdmin) ReverseLedgerEntry(context.Context, account.ReverseLedgerEntryParams) (domain.LedgerEntry, error) {
	return domain.LedgerEntry{}, nil
}

type fakeAdminReminders struct {
	calls          int
	user           domain.User
	text           string
	reconcileCalls int
	reconcileKey   string
}

func (f *fakeAdminReminders) DeliverManual(_ context.Context, user domain.User, _ int64, text string) (bool, error) {
	f.calls++
	f.user, f.text = user, text
	return true, nil
}

func (f *fakeAdminReminders) ConfirmUnknownNotSent(_ context.Context, _ domain.UserID, _ domain.Date, _ domain.ReminderType, deliveryKey string) (bool, error) {
	f.reconcileCalls++
	f.reconcileKey = deliveryKey
	return true, nil
}
