package app

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/Nergous/vpn-balance-bot/internal/config"
	"github.com/Nergous/vpn-balance-bot/internal/localization"
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
	EnableAdmin(int64, telegram.AdminReminderService) error
}

type schedulerRuntime interface{ Start(context.Context) }

type App struct {
	cfg          *config.Config
	logger       *slog.Logger
	newTelegram  func(string, telegram.AccountService, localization.Language, time.Duration, *slog.Logger) (telegramRuntime, error)
	newScheduler func(*billing.Service, *reminder.Service, *time.Location, int, scheduler.Observer) (schedulerRuntime, error)
}

func New(cfg *config.Config, logger *slog.Logger) *App {
	return &App{
		cfg:    cfg,
		logger: logger,
		newTelegram: func(token string, accounts telegram.AccountService, language localization.Language, timeout time.Duration, logger *slog.Logger) (telegramRuntime, error) {
			return telegram.New(token, accounts, language, timeout, logger)
		},
		newScheduler: func(billingService *billing.Service, reminderService *reminder.Service, location *time.Location, hour int, observer scheduler.Observer) (schedulerRuntime, error) {
			return scheduler.New(billingService, reminderService, location, hour, scheduler.WithObserver(observer))
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
	bot, err := a.newTelegram(a.cfg.TelegramBotToken, accounts, a.cfg.BotLanguage, a.cfg.HTTPTimeout, a.logger)
	if err != nil {
		return fmt.Errorf("create Telegram bot: %w", err)
	}
	reminderService, err := reminder.New(s, bot, a.cfg.BotLanguage)
	if err != nil {
		return fmt.Errorf("create reminder service: %w", err)
	}
	if err := bot.EnableAdmin(a.cfg.AdminTelegramID, reminderService); err != nil {
		return fmt.Errorf("enable Telegram admin handlers: %w", err)
	}
	scheduled, err := a.newScheduler(billingService, reminderService, location, a.cfg.ReminderHour, a.schedulerObserver())
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

func (a *App) schedulerObserver() scheduler.Observer {
	return func(result scheduler.Result) {
		attributes := []any{
			slog.String("date", result.Date.String()),
			slog.Int("charges", result.Charges),
			slog.Int("reminders", result.Reminders),
			slog.Bool("skipped", result.Skipped),
		}
		if result.BillingErr != nil {
			attributes = append(attributes, slog.String("billing_error_type", fmt.Sprintf("%T", result.BillingErr)))
		}
		if result.ReminderErr != nil {
			attributes = append(attributes, slog.String("reminder_error_type", fmt.Sprintf("%T", result.ReminderErr)))
		}
		if result.BillingErr != nil || result.ReminderErr != nil {
			a.logger.Error("scheduler run failed", attributes...)
			return
		}
		a.logger.Info("scheduler run completed", attributes...)
	}
}
