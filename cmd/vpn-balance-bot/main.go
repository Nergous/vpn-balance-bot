package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"os/signal"
	"runtime"
	"strings"
	"syscall"

	"github.com/Nergous/vpn-balance-bot/internal/app"
	"github.com/Nergous/vpn-balance-bot/internal/config"
	"github.com/Nergous/vpn-balance-bot/internal/logger"
)

var (
	version   = "dev"
	commit    = "unknown"
	buildDate = "unknown"
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
	case len(args) == 1 && args[0] == "version":
		writeVersion(output)
		return nil
	case len(args) == 1 && args[0] == "doctor":
		cfg, err = config.LoadForMaintenance(ctx)
		if err == nil {
			err = runDoctor(ctx, cfg, output)
		}
		return err
	case len(args) == 1 && args[0] == "migrate-status":
		cfg, err = config.LoadForMaintenance(ctx)
		if err == nil {
			err = runMigrationStatus(ctx, cfg, output)
		}
		return err
	case len(args) == 2 && args[0] == "verify-backup":
		cfg, err = config.LoadForInspection(ctx, args[1])
		if err == nil {
			err = runVerifyBackup(ctx, cfg, output)
		}
		return err
	case len(args) == 2 && args[0] == "backup":
		backupPath = args[1]
		cfg, err = config.LoadForMaintenance(ctx)
	default:
		return fmt.Errorf(
			"usage: vpn-balance-bot [backup <destination>|doctor|migrate-status|verify-backup <path>|version]",
		)
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

func writeVersion(output io.Writer) {
	fmt.Fprintf(output, "vpn-balance-bot %s\n", version)
	fmt.Fprintf(output, "commit: %s\n", commit)
	fmt.Fprintf(output, "built: %s\n", buildDate)
	fmt.Fprintf(output, "go: %s\n", runtime.Version())
	fmt.Fprintf(output, "platform: %s/%s\n", runtime.GOOS, runtime.GOARCH)
}

func runDoctor(ctx context.Context, cfg *config.Config, output io.Writer) error {
	report, err := app.Doctor(ctx, cfg)
	if err != nil {
		return err
	}
	fmt.Fprintln(output, "database: ok")
	fmt.Fprintln(output, "integrity: ok")
	fmt.Fprintf(
		output,
		"migrations: current (%d/%d)\n",
		len(report.Applied),
		len(report.Applied)+len(report.Pending),
	)
	return nil
}

func runMigrationStatus(
	ctx context.Context,
	cfg *config.Config,
	output io.Writer,
) error {
	report, err := app.InspectDatabase(ctx, cfg, cfg.DatabasePath)
	if err != nil {
		return err
	}
	writeMigrationReport(output, report.Applied, report.Pending)
	return nil
}

func runVerifyBackup(
	ctx context.Context,
	cfg *config.Config,
	output io.Writer,
) error {
	report, err := app.InspectDatabase(ctx, cfg, cfg.DatabasePath)
	if err != nil {
		return err
	}
	fmt.Fprintln(output, "backup: ok")
	fmt.Fprintln(output, "integrity: ok")
	writeMigrationReport(output, report.Applied, report.Pending)
	return nil
}

func writeMigrationReport(output io.Writer, applied, pending []string) {
	total := len(applied) + len(pending)
	if len(pending) == 0 {
		fmt.Fprintf(output, "migrations: current (%d/%d)\n", len(applied), total)
		return
	}
	fmt.Fprintf(
		output,
		"migrations: compatible (%d/%d applied)\n",
		len(applied),
		total,
	)
	fmt.Fprintf(output, "pending: %s\n", strings.Join(pending, ", "))
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
