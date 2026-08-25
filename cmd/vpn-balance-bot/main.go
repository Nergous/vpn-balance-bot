package main

import (
	"context"
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

	cfg := config.MustLoad(ctx)

	logger, err := logger.New(cfg, os.Stdout)
	if err != nil {
		panic(err)
	}

	slog.SetDefault(logger)

	application := app.New(cfg, logger)

	if err := application.Run(ctx); err != nil {
		logger.Error(
			"application stopped",
			slog.Any("error", err),
		)
	}
}
