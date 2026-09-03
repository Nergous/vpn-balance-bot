package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"github.com/Nergous/vpn-balance-bot/internal/app"
	"github.com/Nergous/vpn-balance-bot/internal/config"
	"github.com/Nergous/vpn-balance-bot/internal/logger"
)

func main() {
	ctx, stop := signal.NotifyContext(
		context.Background(),
		os.Interrupt,
		syscall.SIGTERM,
	)
	defer stop()

	if err := run(ctx, os.Args[1:], os.Stdout); err != nil {
		logCommandFailure(slog.Default(), err)
		os.Exit(1)
	}
}

func run(ctx context.Context, args []string, output io.Writer) error {
	var (
		cfg        *config.Config
		err        error
		backupPath string
	)

	switch {
	case len(args) == 0:
		cfg, err = config.Load(ctx)
	case len(args) == 2 && args[0] == "backup":
		backupPath = args[1]
		cfg, err = config.LoadForMaintenance(ctx)
	default:
		return fmt.Errorf("usage: vpn-balance-bot [backup <destination>]")
	}
	if err != nil {
		return err
	}

	applicationLogger, err := logger.New(cfg, output)
	if err != nil {
		return err
	}
	slog.SetDefault(applicationLogger)

	if backupPath != "" {
		if err := app.Backup(ctx, cfg, backupPath); err != nil {
			return fmt.Errorf("backup database: %w", err)
		}
		applicationLogger.Info("backup completed")
		return nil
	}

	application := app.New(cfg, applicationLogger)
	if err := application.Run(ctx); err != nil {
		return fmt.Errorf("run application: %w", err)
	}
	return nil
}

func logCommandFailure(log *slog.Logger, err error) {
	if err == nil {
		return
	}
	if log == nil {
		log = slog.Default()
	}
	log.Error("command failed",
		slog.String("error_type", fmt.Sprintf("%T", err)),
		slog.String("error_kind", commandErrorKind(err)),
	)
}

func commandErrorKind(err error) string {
	switch {
	case errors.Is(err, context.Canceled):
		return "canceled"
	case errors.Is(err, context.DeadlineExceeded):
		return "timeout"
	default:
		return "failed"
	}
}
