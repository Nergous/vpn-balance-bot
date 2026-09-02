package telegram

import (
	"context"
	"errors"
	"strings"
	"testing"

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
	wizard.BeginPayment(1, PaymentDraft{UserID: 7, AmountMinor: 100})
	if err := wizard.ConfirmPayment(context.Background(), 1, service); err != nil {
		t.Fatal(err)
	}
	if service.paymentCalls != 1 {
		t.Fatalf("payment calls=%d", service.paymentCalls)
	}
	if err := wizard.ConfirmPayment(context.Background(), 1, service); err != ErrWizardNotFound {
		t.Fatalf("second confirmation=%v", err)
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
	if len(client.Sent) != 2 || !strings.Contains(client.Sent[0].Text, "Users: 4") || !strings.Contains(client.Sent[1].Text, "/admin users all 9") {
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
	if err := bot.HandleAdminCommand(context.Background(), IncomingMessage{ChatID: 1, UserID: 1, ChatType: ChatTypePrivate, Text: "/admin confirm"}); !errors.Is(err, ErrWizardNotFound) {
		t.Fatalf("second confirmation = %v", err)
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

type fakeAdmin struct{ pauseCalls, paymentCalls int }

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
func (f *fakeAdmin) CreateUser(context.Context, account.CreateUserParams) (domain.User, error) {
	return domain.User{}, nil
}
func (f *fakeAdmin) UserByID(_ context.Context, userID domain.UserID) (domain.User, error) {
	return domain.User{ID: userID}, nil
}
func (f *fakeAdmin) ListUsers(context.Context, account.UserFilter) ([]domain.User, error) {
	return nil, nil
}
func (f *fakeAdmin) CreateInviteToken(context.Context, domain.UserID) (string, error) {
	return "token", nil
}
func (f *fakeAdmin) ChangeMonthlyFee(context.Context, account.ChangeMonthlyFeeParams) (domain.User, error) {
	return domain.User{}, nil
}
func (f *fakeAdmin) Pause(context.Context, domain.UserID) (domain.User, error) {
	f.pauseCalls++
	return domain.User{}, nil
}
func (f *fakeAdmin) Resume(context.Context, account.ResumeParams) (domain.User, error) {
	return domain.User{}, nil
}
func (f *fakeAdmin) Disable(context.Context, domain.UserID) (domain.User, error) {
	return domain.User{}, nil
}
func (f *fakeAdmin) AddPayment(context.Context, account.AddPaymentParams) (domain.LedgerEntry, error) {
	f.paymentCalls++
	return domain.LedgerEntry{}, nil
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
	calls int
	user  domain.User
	text  string
}

func (f *fakeAdminReminders) DeliverManual(_ context.Context, user domain.User, text string) (bool, error) {
	f.calls++
	f.user, f.text = user, text
	return true, nil
}
