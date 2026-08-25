package logger

import (
	"io"
	"log/slog"

	"github.com/Nergous/vpn-balance-bot/internal/config"
)

func New(cfg *config.Config, output io.Writer) (*slog.Logger, error) {
	var level slog.Level

	if err := level.UnmarshalText([]byte(cfg.LogLevel)); err != nil {
		return nil, err
	}

	options := &slog.HandlerOptions{
		Level:     level,
		AddSource: cfg.AppEnv == config.EnvDevelopment,
	}

	var handler slog.Handler
	if cfg.AppEnv == config.EnvProduction {
		handler = slog.NewJSONHandler(output, options)
	} else {
		handler = slog.NewTextHandler(output, options)
	}

	return slog.New(handler), nil
}
