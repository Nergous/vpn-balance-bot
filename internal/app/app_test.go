package app

import (
	"context"
	"io"
	"log/slog"
	"path/filepath"
	"testing"
	"time"

	"github.com/Nergous/vpn-balance-bot/internal/config"
	"github.com/Nergous/vpn-balance-bot/internal/localization"
	"github.com/Nergous/vpn-balance-bot/internal/service/billing"
	"github.com/Nergous/vpn-balance-bot/internal/service/reminder"
	"github.com/Nergous/vpn-balance-bot/internal/service/scheduler"
	"github.com/Nergous/vpn-balance-bot/internal/telegram"
)

func TestRunMigratesTemporaryDatabaseAndStopsWorkers(t *testing.T) {
	cfg := &config.Config{
		TelegramBotToken: "test-token", DatabasePath: filepath.Join(t.TempDir(), "bot.db"),
		DBTimeout: time.Second, HTTPTimeout: 7 * time.Second, InviteTTL: time.Hour, AppTimezone: "UTC", ReminderHour: 9,
		BotLanguage: localization.Russian,
	}
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	application := New(cfg, logger)
	bot := &fakeTelegram{started: make(chan struct{})}
	scheduled := &fakeScheduler{started: make(chan struct{})}
	application.newTelegram = func(_ string, _ telegram.AccountService, _ localization.Language, timeout time.Duration, gotLogger *slog.Logger) (telegramRuntime, error) {
		if timeout != cfg.HTTPTimeout || gotLogger != logger {
			t.Fatalf("Telegram config: timeout=%s logger_match=%t", timeout, gotLogger == logger)
		}
		return bot, nil
	}
	application.newScheduler = func(*billing.Service, *reminder.Service, *time.Location, int, scheduler.Observer) (schedulerRuntime, error) {
		return scheduled, nil
	}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- application.Run(ctx) }()
	select {
	case <-bot.started:
	case <-time.After(time.Second):
		t.Fatal("Telegram worker did not start")
	}
	select {
	case <-scheduled.started:
	case <-time.After(time.Second):
		t.Fatal("scheduler worker did not start")
	}
	cancel()
	if err := <-done; err != nil {
		t.Fatal(err)
	}
}

type fakeTelegram struct {
	started chan struct{}
}

func (f *fakeTelegram) Start(ctx context.Context) {
	close(f.started)
	<-ctx.Done()
}
func (f *fakeTelegram) SendReminder(context.Context, int64, string) (int, error) { return 0, nil }
func (f *fakeTelegram) EnableAdmin(int64, telegram.AdminReminderService) error   { return nil }

type fakeScheduler struct {
	started chan struct{}
}

func (f *fakeScheduler) Start(ctx context.Context) {
	close(f.started)
	<-ctx.Done()
}

var _ telegramRuntime = (*fakeTelegram)(nil)
var _ schedulerRuntime = (*fakeScheduler)(nil)
