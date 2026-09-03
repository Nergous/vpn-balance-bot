package main

import (
	"bytes"
	"context"
	"errors"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Nergous/vpn-balance-bot/internal/storage/sqlite"
)

func TestRunBackupCommand(t *testing.T) {
	ctx := context.Background()
	source := filepath.Join(t.TempDir(), "source.db")
	store, err := sqlite.New(ctx, source, 5*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.Migrate(ctx); err != nil {
		t.Fatal(err)
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}

	t.Setenv("APP_ENV", "test")
	t.Setenv("TELEGRAM_BOT_TOKEN", "test-token")
	t.Setenv("ADMIN_TELEGRAM_ID", "1")
	t.Setenv("DATABASE_PATH", source)
	t.Setenv("HTTP_TIMEOUT", "2s")
	t.Setenv("DB_TIMEOUT", "5s")
	destination := filepath.Join(t.TempDir(), "snapshot.db")

	if err := run(ctx, []string{"backup", destination}, io.Discard); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(destination); err != nil {
		t.Fatal(err)
	}
}

func TestRunRejectsInvalidCommand(t *testing.T) {
	if err := run(context.Background(), []string{"unknown"}, io.Discard); err == nil {
		t.Fatal("run() accepted an unknown command")
	}
}

func TestCommandFailureLogDoesNotExposeFilesystemPathOrErrorText(t *testing.T) {
	var output bytes.Buffer
	log := slog.New(slog.NewJSONHandler(&output, nil))
	sensitivePath := `D:\sensitive\customer-ledger.db`
	logCommandFailure(log, errors.New("open "+sensitivePath+": access denied"))

	got := output.String()
	for _, forbidden := range []string{sensitivePath, "customer-ledger.db", "access denied"} {
		if strings.Contains(got, forbidden) {
			t.Fatalf("log exposed %q: %s", forbidden, got)
		}
	}
	for _, expected := range []string{`"msg":"command failed"`, `"error_type":"*errors.errorString"`, `"error_kind":"failed"`} {
		if !strings.Contains(got, expected) {
			t.Fatalf("log missing %q: %s", expected, got)
		}
	}
}
