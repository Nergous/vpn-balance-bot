package telegram

import (
	"context"
	"errors"
	"testing"

	"github.com/Nergous/vpn-balance-bot/internal/domain"
	"github.com/Nergous/vpn-balance-bot/internal/service/account"
	"github.com/Nergous/vpn-balance-bot/internal/testutil"
)

func TestAdminRejectsNonAdminBeforeMutation(t *testing.T) {
	client := &testutil.FakeTelegramClient{}
	service := &fakeAdmin{}
	admin := NewAdmin(client, service, 1)
	if err := admin.Pause(context.Background(), IncomingMessage{ChatID: 2, UserID: 2}, 7); err != nil {
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
	admin := NewAdmin(client, service, 1)
	if err := admin.Dashboard(context.Background(), IncomingMessage{ChatID: 1, UserID: 1}); err != nil {
		t.Fatal(err)
	}
	if _, err := admin.CreateUser(context.Background(), IncomingMessage{UserID: 2}, account.CreateUserParams{}); !errors.Is(err, ErrAdminOnly) {
		t.Fatalf("CreateUser error=%v", err)
	}
}

func TestAdminCommandRouterAuthorizesAndConfirmsPaymentOnce(t *testing.T) {
	client := &testutil.FakeTelegramClient{}
	service := &fakeAdmin{}
	bot := NewWithClient(client, service)
	if err := bot.EnableAdmin(1); err != nil {
		t.Fatal(err)
	}
	if err := bot.HandleAdminCommand(context.Background(), IncomingMessage{ChatID: 2, UserID: 2, Text: "/admin payment 7 100"}); err != nil {
		t.Fatal(err)
	}
	if service.paymentCalls != 0 {
		t.Fatal("non-admin command reached payment service")
	}
	if err := bot.HandleAdminCommand(context.Background(), IncomingMessage{ChatID: 1, UserID: 1, Text: "/admin payment 7 100 note"}); err != nil {
		t.Fatal(err)
	}
	if err := bot.HandleAdminCommand(context.Background(), IncomingMessage{ChatID: 1, UserID: 1, Text: "/admin confirm"}); err != nil {
		t.Fatal(err)
	}
	if service.paymentCalls != 1 {
		t.Fatalf("payment calls = %d", service.paymentCalls)
	}
	if err := bot.HandleAdminCommand(context.Background(), IncomingMessage{ChatID: 1, UserID: 1, Text: "/admin confirm"}); !errors.Is(err, ErrWizardNotFound) {
		t.Fatalf("second confirmation = %v", err)
	}
}

func TestAdminManualReminderRequiresAdminAndUsesReminderService(t *testing.T) {
	client := &testutil.FakeTelegramClient{}
	service := &fakeAdmin{}
	reminders := &fakeAdminReminders{}
	admin := NewAdmin(client, service, 1, reminders)
	if err := admin.RemindNow(context.Background(), IncomingMessage{ChatID: 2, UserID: 2}, 7, "top up"); err != nil {
		t.Fatal(err)
	}
	if reminders.calls != 0 {
		t.Fatal("non-admin reached reminder service")
	}
	if err := admin.RemindNow(context.Background(), IncomingMessage{ChatID: 1, UserID: 1}, 7, "top up"); err != nil {
		t.Fatal(err)
	}
	if reminders.calls != 1 || reminders.user.ID != 7 || reminders.text != "top up" {
		t.Fatalf("reminder = %#v", reminders)
	}
}

type fakeAdmin struct{ pauseCalls, paymentCalls int }

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
