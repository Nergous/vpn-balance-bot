package app

import (
	"context"
	"log/slog"

	"github.com/Nergous/vpn-balance-bot/internal/config"
)

type App struct {
	cfg    *config.Config
	logger *slog.Logger
}

func New(cfg *config.Config, logger *slog.Logger) *App {
	return &App{
		cfg:    cfg,
		logger: logger,
	}
}

func (a *App) Run(ctx context.Context) error {
	return nil
}
