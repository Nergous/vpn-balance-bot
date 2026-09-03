package telegram

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
	"net"
	"strings"
	"testing"
	"time"

	"github.com/Nergous/vpn-balance-bot/internal/domain"
	"github.com/Nergous/vpn-balance-bot/internal/service/account"
	"github.com/Nergous/vpn-balance-bot/internal/service/reminder"
	"github.com/Nergous/vpn-balance-bot/internal/testutil"
	botapi "github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"
)

func TestClassifyReminderErrorRetriesOnlyDefiniteRateLimitRejections(t *testing.T) {
	bot := &Bot{}
	tests := []struct {
		name string
		err  error
		want reminder.DeliveryErrorCode
	}{
		{name: "forbidden", err: botapi.ErrorForbidden, want: reminder.DeliveryErrorOffline},
		{name: "rate limited", err: &botapi.TooManyRequestsError{Message: "retry later", RetryAfter: 1}, want: reminder.DeliveryErrorRetryable},
		{name: "deadline after uncertain send", err: context.DeadlineExceeded, want: reminder.DeliveryErrorUnknown},
		{name: "temporary DNS failure", err: &net.DNSError{Err: "temporary", IsTemporary: true}, want: reminder.DeliveryErrorRetryable},
		{name: "generic transport failure", err: errors.New("transport failed"), want: reminder.DeliveryErrorFailed},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := bot.ClassifyReminderError(test.err); got != test.want {
				t.Fatalf("ClassifyReminderError() = %q, want %q", got, test.want)
			}
		})
	}
}

func TestUserHandlersRejectSensitiveRequestsOutsidePrivateChat(t *testing.T) {
	for _, chatType := range []ChatType{ChatTypeGroup, ChatTypeSupergroup} {
		t.Run(string(chatType), func(t *testing.T) {
			consumeCalls := 0
			lookupCalls := 0
			service := &fakeAccount{
				consume: func(context.Context, account.ConsumeInviteParams) (domain.User, error) {
					consumeCalls++
					return domain.User{}, nil
				},
				byTelegram: func(context.Context, int64) (domain.User, error) {
					lookupCalls++
					return domain.User{}, nil
				},
			}
			client := &testutil.FakeTelegramClient{}
			bot := NewWithClient(client, service, LanguageEnglish)

			requests := []struct {
				name string
				run  func() error
			}{
				{"invite", func() error {
					return bot.HandleStart(context.Background(), IncomingMessage{ChatID: -100, UserID: 11, ChatType: chatType, Text: "/start secret-token"})
				}},
				{"status", func() error {
					return bot.HandleStatus(context.Background(), IncomingMessage{ChatID: -100, UserID: 11, ChatType: chatType})
				}},
				{"history", func() error {
					return bot.HandleHistory(context.Background(), IncomingMessage{ChatID: -100, UserID: 11, ChatType: chatType})
				}},
			}

			for _, request := range requests {
				t.Run(request.name, func(t *testing.T) {
					if err := request.run(); !errors.Is(err, ErrPrivateChatRequired) {
						t.Fatalf("error = %v", err)
					}
				})
			}
			if consumeCalls != 0 || lookupCalls != 0 || len(client.Sent) != 0 {
				t.Fatalf("consume=%d lookup=%d sent=%#v", consumeCalls, lookupCalls, client.Sent)
			}
		})
	}
}

func TestAdminCommandsRejectConfiguredAdminOutsidePrivateChat(t *testing.T) {
	for _, chatType := range []ChatType{ChatTypeGroup, ChatTypeSupergroup} {
		t.Run(string(chatType), func(t *testing.T) {
			client := &testutil.FakeTelegramClient{}
			service := &fakeAdmin{}
			bot := NewWithClient(client, service, LanguageEnglish)
			if err := bot.EnableAdmin(1, &fakeAdminReminders{}); err != nil {
				t.Fatal(err)
			}

			err := bot.HandleAdminCommand(context.Background(), IncomingMessage{
				ChatID: -100, UserID: 1, ChatType: chatType, Text: "/admin payment 7 100 secret-note",
			})
			if err != nil {
				t.Fatal(err)
			}
			if service.paymentCalls != 0 {
				t.Fatal("group admin command reached account service")
			}
			if len(client.Sent) != 1 || client.Sent[0].ChatID != -100 || strings.Contains(client.Sent[0].Text, "secret-note") {
				t.Fatalf("sent = %#v", client.Sent)
			}
		})
	}
}

func TestStatusRejectsStoredTelegramChatIDMismatch(t *testing.T) {
	storedChatID := int64(22)
	balanceCalls := 0
	service := &fakeAccount{
		byTelegram: func(context.Context, int64) (domain.User, error) {
			return domain.User{ID: 7, TelegramChatID: &storedChatID}, nil
		},
		balance: func(context.Context, domain.UserID) (domain.AmountMinor, error) {
			balanceCalls++
			return 100, nil
		},
	}
	client := &testutil.FakeTelegramClient{}
	bot := NewWithClient(client, service, LanguageEnglish)

	if err := bot.HandleStatus(context.Background(), IncomingMessage{ChatID: 23, UserID: 11, ChatType: ChatTypePrivate}); err != nil {
		t.Fatal(err)
	}
	if balanceCalls != 0 {
		t.Fatal("chat mismatch reached balance lookup")
	}
	if len(client.Sent) != 1 || !strings.Contains(client.Sent[0].Text, "Profile is not linked") {
		t.Fatalf("sent = %#v", client.Sent)
	}
}

func TestStatusUsesLastUnreversedPaymentCapability(t *testing.T) {
	chatID := int64(22)
	nextCharge, err := domain.NewDate(2026, time.September, 10)
	if err != nil {
		t.Fatal(err)
	}
	base := &fakeAccount{
		byTelegram: func(context.Context, int64) (domain.User, error) {
			return domain.User{
				ID: 7, TelegramChatID: &chatID, DisplayName: "Alice", Currency: "RUB",
				MonthlyFeeMinor: 100000, NextChargeOn: nextCharge,
			}, nil
		},
		balance: func(context.Context, domain.UserID) (domain.AmountMinor, error) { return 0, nil },
		entries: func(context.Context, domain.UserID) ([]domain.LedgerEntry, error) {
			t.Fatal("status fell back to LastLedgerEntries")
			return nil, nil
		},
	}
	service := &fakeAccountWithLastPayment{
		fakeAccount: base,
		lastPayment: func(_ context.Context, userID domain.UserID) (domain.LedgerEntry, bool, error) {
			if userID != 7 {
				t.Fatalf("user ID = %d", userID)
			}
			return domain.LedgerEntry{Kind: domain.LedgerKindPayment, AmountMinor: 54321}, true, nil
		},
	}
	client := &testutil.FakeTelegramClient{}
	bot := NewWithClient(client, service, LanguageEnglish)

	if err := bot.HandleStatus(context.Background(), IncomingMessage{ChatID: chatID, UserID: 11, ChatType: ChatTypePrivate}); err != nil {
		t.Fatal(err)
	}
	if service.calls != 1 || len(client.Sent) != 1 || !strings.Contains(client.Sent[0].Text, "543.21 RUB") {
		t.Fatalf("calls=%d sent=%#v", service.calls, client.Sent)
	}
}

func TestHandlerErrorReportsCorrelationWithoutSensitiveInput(t *testing.T) {
	const (
		messageText  = "/status secret-token"
		username     = "secret-username"
		callbackData = "raw-callback-payload"
	)

	var output bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&output, nil))
	client := &testutil.FakeTelegramClient{Err: errors.New("transport secret-token")}
	service := &fakeAccount{byTelegram: func(context.Context, int64) (domain.User, error) {
		return domain.User{}, account.ErrNotFound
	}}
	bot := NewWithClient(client, service, LanguageEnglish)
	bot.logger = logger

	bot.statusUpdate(context.Background(), nil, &models.Update{
		ID: 42,
		Message: &models.Message{
			ID:   9,
			Chat: models.Chat{ID: 22, Type: models.ChatTypePrivate},
			From: &models.User{ID: 11, Username: username},
			Text: messageText,
		},
	})
	bot.reportCallbackError("handle", IncomingCallback{
		UpdateID: 43, MessageID: 10, CallbackQueryID: "callback-44",
		ChatID: 22, UserID: 11, ChatType: ChatTypePrivate, Data: callbackData,
	}, errors.New("callback secret-token"))

	logOutput := output.String()
	for _, expected := range []string{
		`"operation":"status"`, `"update_id":42`, `"message_id":9`,
		`"operation":"handle"`, `"update_id":43`, `"callback_query_id":"callback-44"`,
	} {
		if !strings.Contains(logOutput, expected) {
			t.Fatalf("log missing %q: %s", expected, logOutput)
		}
	}
	for _, secret := range []string{messageText, "secret-token", username, callbackData} {
		if strings.Contains(logOutput, secret) {
			t.Fatalf("log contains sensitive input %q: %s", secret, logOutput)
		}
	}
}

func TestProductionUpdateErrorReporterKeepsCorrelationWithoutMessageContents(t *testing.T) {
	var output bytes.Buffer
	bot := NewWithClient(&testutil.FakeTelegramClient{}, &fakeAdmin{}, LanguageEnglish)
	bot.logger = slog.New(slog.NewJSONHandler(&output, nil))
	secret := "/admin pause 7 private-note"
	bot.reportUpdateError(&models.Update{
		ID: 77,
		Message: &models.Message{
			ID: 9, Chat: models.Chat{ID: 22, Type: models.ChatTypePrivate},
			From: &models.User{ID: 11, Username: "private-user"}, Text: secret,
		},
	}, errors.New("database private-note"))

	got := output.String()
	for _, expected := range []string{`"operation":"admin"`, `"update_id":77`, `"message_id":9`, `"chat_id":22`, `"telegram_user_id":11`} {
		if !strings.Contains(got, expected) {
			t.Fatalf("log missing %q: %s", expected, got)
		}
	}
	for _, forbidden := range []string{secret, "private-note", "private-user"} {
		if strings.Contains(got, forbidden) {
			t.Fatalf("log exposed %q: %s", forbidden, got)
		}
	}
}

type fakeAccountWithLastPayment struct {
	*fakeAccount
	lastPayment func(context.Context, domain.UserID) (domain.LedgerEntry, bool, error)
	calls       int
}

func (f *fakeAccountWithLastPayment) LastUnreversedPayment(ctx context.Context, userID domain.UserID) (domain.LedgerEntry, bool, error) {
	f.calls++
	return f.lastPayment(ctx, userID)
}
