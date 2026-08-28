package telegram

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/Nergous/vpn-balance-bot/internal/domain"
	"github.com/Nergous/vpn-balance-bot/internal/service/account"
	"github.com/Nergous/vpn-balance-bot/internal/testutil"
)

func TestUserHandlersDoNotExposeUnlinkedProfiles(t *testing.T) {
	client := &testutil.FakeTelegramClient{}
	service := &fakeAccount{byTelegram: func(context.Context, int64) (domain.User, error) { return domain.User{}, account.ErrNotFound }}
	bot := NewWithClient(client, service)
	if err := bot.HandleStatus(context.Background(), IncomingMessage{ChatID: 1, UserID: 999, Username: "spoofed"}); err != nil {
		t.Fatal(err)
	}
	if len(client.Sent) != 1 || !strings.Contains(client.Sent[0].Text, "не привязан") {
		t.Fatalf("sent = %#v", client.Sent)
	}
}

func TestStartInviteAndOwnStatus(t *testing.T) {
	client := &testutil.FakeTelegramClient{}
	date, _ := domain.NewDate(2026, time.September, 1)
	user := domain.User{ID: 7, DisplayName: "Alice", MonthlyFeeMinor: 100000, Currency: "RUB", NextChargeOn: date}
	service := &fakeAccount{
		consume: func(_ context.Context, params account.ConsumeInviteParams) (domain.User, error) {
			if params.Token != "token" || params.TelegramUserID != 11 || params.TelegramChatID != 22 {
				t.Fatalf("consume params = %#v", params)
			}
			return user, nil
		},
		byTelegram: func(_ context.Context, id int64) (domain.User, error) {
			if id != 11 {
				t.Fatalf("lookup ID = %d", id)
			}
			return user, nil
		},
		balance: func(context.Context, domain.UserID) (domain.AmountMinor, error) { return -25000, nil },
		entries: func(context.Context, domain.UserID) ([]domain.LedgerEntry, error) {
			return []domain.LedgerEntry{{Kind: domain.LedgerKindPayment, AmountMinor: 50000}}, nil
		},
	}
	bot := NewWithClient(client, service)
	if err := bot.HandleStart(context.Background(), IncomingMessage{ChatID: 22, UserID: 11, Text: "/start token"}); err != nil {
		t.Fatal(err)
	}
	if err := bot.HandleStatus(context.Background(), IncomingMessage{ChatID: 22, UserID: 11, Username: "another-name"}); err != nil {
		t.Fatal(err)
	}
	if len(client.Sent) != 2 || !strings.Contains(client.Sent[1].Text, "Долг: 25000 RUB") || !strings.Contains(client.Sent[1].Text, "Alice") {
		t.Fatalf("sent = %#v", client.Sent)
	}
}

func TestHistoryUsesAtMostTenEntries(t *testing.T) {
	client := &testutil.FakeTelegramClient{}
	date, _ := domain.NewDate(2026, time.September, 1)
	entries := make([]domain.LedgerEntry, 10)
	for i := range entries {
		entries[i] = domain.LedgerEntry{ID: int64(i + 1), Kind: domain.LedgerKindPayment, AmountMinor: domain.AmountMinor(i + 1), OccurredAt: time.Date(2026, time.August, 28, 0, 0, i, 0, time.UTC)}
	}
	service := &fakeAccount{byTelegram: func(context.Context, int64) (domain.User, error) { return domain.User{ID: 1, NextChargeOn: date}, nil }, entries: func(context.Context, domain.UserID) ([]domain.LedgerEntry, error) { return entries, nil }}
	bot := NewWithClient(client, service)
	if err := bot.HandleHistory(context.Background(), IncomingMessage{ChatID: 1, UserID: 1}); err != nil {
		t.Fatal(err)
	}
	if strings.Count(client.Sent[0].Text, "payment") != 10 {
		t.Fatalf("history = %q", client.Sent[0].Text)
	}
}

type fakeAccount struct {
	consume    func(context.Context, account.ConsumeInviteParams) (domain.User, error)
	byTelegram func(context.Context, int64) (domain.User, error)
	balance    func(context.Context, domain.UserID) (domain.AmountMinor, error)
	entries    func(context.Context, domain.UserID) ([]domain.LedgerEntry, error)
}

func (f *fakeAccount) ConsumeInviteToken(ctx context.Context, p account.ConsumeInviteParams) (domain.User, error) {
	if f.consume == nil {
		return domain.User{}, errors.New("unexpected consume")
	}
	return f.consume(ctx, p)
}
func (f *fakeAccount) UserByTelegramID(ctx context.Context, id int64) (domain.User, error) {
	if f.byTelegram == nil {
		return domain.User{}, errors.New("unexpected lookup")
	}
	return f.byTelegram(ctx, id)
}
func (f *fakeAccount) Balance(ctx context.Context, id domain.UserID) (domain.AmountMinor, error) {
	if f.balance == nil {
		return 0, errors.New("unexpected balance")
	}
	return f.balance(ctx, id)
}
func (f *fakeAccount) LastLedgerEntries(ctx context.Context, id domain.UserID) ([]domain.LedgerEntry, error) {
	if f.entries == nil {
		return nil, errors.New("unexpected entries")
	}
	return f.entries(ctx, id)
}
