package app

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/Nergous/vpn-balance-bot/internal/config"
	"github.com/Nergous/vpn-balance-bot/internal/service/account"
	"github.com/Nergous/vpn-balance-bot/internal/service/billing"
	"github.com/Nergous/vpn-balance-bot/internal/service/reminder"
	"github.com/Nergous/vpn-balance-bot/internal/service/scheduler"
	"github.com/Nergous/vpn-balance-bot/internal/storage/sqlite"
	"github.com/Nergous/vpn-balance-bot/internal/telegram"
)

type telegramRuntime interface {
	reminder.Sender
	Start(context.Context)
}

type schedulerRuntime interface{ Start(context.Context) }

type App struct {
	cfg          *config.Config
	logger       *slog.Logger
	newTelegram  func(string, telegram.AccountService) (telegramRuntime, error)
	newScheduler func(*billing.Service, *reminder.Service, *time.Location, int) (schedulerRuntime, error)
}

func New(cfg *config.Config, logger *slog.Logger) *App {
	return &App{
		cfg:    cfg,
		logger: logger,
		newTelegram: func(token string, accounts telegram.AccountService) (telegramRuntime, error) {
			return telegram.New(token, accounts)
		},
		newScheduler: func(billingService *billing.Service, reminderService *reminder.Service, location *time.Location, hour int) (schedulerRuntime, error) {
			return scheduler.New(billingService, reminderService, location, hour)
		},
	}
}

func (a *App) Run(ctx context.Context) error {
	if a.cfg == nil {
		return fmt.Errorf("application config is nil")
	}
	if a.logger == nil {
		return fmt.Errorf("application logger is nil")
	}
	if ctx == nil {
		return fmt.Errorf("application context is nil")
	}

	s, err := sqlite.New(ctx, a.cfg.DatabasePath, a.cfg.DBTimeout)
	if err != nil {
		return err
	}
	defer s.Close()

	if err := s.Migrate(ctx); err != nil {
		return err
	}
	location, err := time.LoadLocation(a.cfg.AppTimezone)
	if err != nil {
		return fmt.Errorf("load application timezone: %w", err)
	}
	accounts, err := account.New(s, a.cfg.InviteTTL)
	if err != nil {
		return fmt.Errorf("create account service: %w", err)
	}
	billingService, err := billing.New(s)
	if err != nil {
		return fmt.Errorf("create billing service: %w", err)
	}
	bot, err := a.newTelegram(a.cfg.TelegramBotToken, accounts)
	if err != nil {
		return fmt.Errorf("create Telegram bot: %w", err)
	}
	reminderService, err := reminder.New(s, bot)
	if err != nil {
		return fmt.Errorf("create reminder service: %w", err)
	}
	scheduled, err := a.newScheduler(billingService, reminderService, location, a.cfg.ReminderHour)
	if err != nil {
		return fmt.Errorf("create scheduler: %w", err)
	}

	a.logger.Info("application started")
	var workers sync.WaitGroup
	workers.Add(2)
	go func() {
		defer workers.Done()
		bot.Start(ctx)
	}()
	go func() {
		defer workers.Done()
		scheduled.Start(ctx)
	}()

	<-ctx.Done()
	workers.Wait()
	a.logger.Info("application stopped", slog.String("reason", ctx.Err().Error()))

	return nil
}
