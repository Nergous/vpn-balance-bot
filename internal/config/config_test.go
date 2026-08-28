package config

import (
	"context"
	"errors"
	"io"
	"net/http"
	"os"
	"strings"
	"testing"
	"time"
)

func TestLoadAndMustLoad(t *testing.T) {
	setBaseEnvironment(t, EnvTest)
	t.Setenv("TELEGRAM_BOT_TOKEN", "  test-token  ")
	cfg, err := Load(context.Background())
	if err != nil || cfg.TelegramBotToken != "test-token" || cfg.AdminTelegramID != 12345 ||
		cfg.DatabasePath != "./data/test.db" || cfg.AppTimezone != defaultTimezone ||
		cfg.ReminderHour != 9 || cfg.InviteTTL != 168*time.Hour || cfg.LogLevel != InfoLevel {
		t.Fatalf("Load() = %+v, %v", cfg, err)
	}
	if MustLoad(context.Background()).AppEnv != EnvTest {
		t.Error("MustLoad() returned wrong environment")
	}

	t.Run("panic", func(t *testing.T) {
		setBaseEnvironment(t, EnvTest)
		t.Setenv("ADMIN_TELEGRAM_ID", "")
		defer func() {
			if recover() == nil {
				t.Error("MustLoad() did not panic")
			}
		}()
		MustLoad(context.Background())
	})
}

func TestLoadProduction(t *testing.T) {
	setBaseEnvironment(t, EnvProduction)
	t.Setenv("TELEGRAM_BOT_TOKEN", "production-token")
	t.Setenv("DATABASE_PATH", "/var/lib/vpn-balance-bot/bot.db")
	t.Setenv("APP_TIMEZONE", "UTC")
	t.Setenv("REMINDER_HOUR", "4")
	t.Setenv("INVITE_TTL", "24h")
	t.Setenv("LOG_LEVEL", DebugLevel)
	cfg, err := Load(context.Background())
	if err != nil || cfg.AppEnv != EnvProduction || cfg.ReminderHour != 4 ||
		cfg.InviteTTL != 24*time.Hour || cfg.LogLevel != DebugLevel {
		t.Fatalf("Load() = %+v, %v", cfg, err)
	}
}

func TestLoadErrors(t *testing.T) {
	t.Setenv("APP_ENV", "staging")
	if _, err := Load(context.Background()); !errors.Is(err, ErrInvalidAppEnv) {
		t.Fatalf("invalid APP_ENV error = %v", err)
	}

	setBaseEnvironment(t, EnvTest)
	t.Setenv("ADMIN_TELEGRAM_ID", "bad")
	if _, err := Load(context.Background()); !errors.Is(err, ErrInvalidEnvironmentValue) {
		t.Fatalf("invalid admin ID error = %v", err)
	}

	setBaseEnvironment(t, EnvTest)
	t.Setenv("ADMIN_TELEGRAM_ID", "")
	if _, err := Load(context.Background()); !errors.Is(err, ErrRequiredEnvironmentVariable) {
		t.Fatalf("missing admin ID error = %v", err)
	}

	setBaseEnvironment(t, EnvTest)
	t.Setenv("APP_TIMEZONE", "Invalid/Timezone")
	if _, err := Load(context.Background()); !errors.Is(err, ErrInvalidTimezone) {
		t.Fatalf("invalid timezone error = %v", err)
	}

}

func TestLoadDevelopmentEnvironment(t *testing.T) {
	t.Chdir(t.TempDir())
	unsetEnv(t, "APP_ENV")
	if err := os.WriteFile(".env", []byte("APP_ENV=development\nTELEGRAM_BOT_TOKEN=dev-token\nADMIN_TELEGRAM_ID=123\nDATABASE_PATH=data/dev.db\n"), 0600); err != nil {
		t.Fatal(err)
	}
	cfg, err := Load(context.Background())
	if err != nil || cfg.AppEnv != EnvDevelopment || cfg.TelegramBotToken != "dev-token" {
		t.Fatalf("Load() = %+v, %v", cfg, err)
	}
}

func TestLoadEnvironmentFileErrors(t *testing.T) {
	t.Run("file error", func(t *testing.T) {
		t.Chdir(t.TempDir())
		unsetEnv(t, "APP_ENV")
		if err := os.Mkdir(".env", 0700); err != nil {
			t.Fatal(err)
		}
		if _, err := Load(context.Background()); !errors.Is(err, ErrEnvironmentFile) {
			t.Fatalf("error = %v", err)
		}
	})

	t.Run("invalid app env from file", func(t *testing.T) {
		t.Chdir(t.TempDir())
		unsetEnv(t, "APP_ENV")
		if err := os.WriteFile(".env", []byte("APP_ENV=staging\n"), 0600); err != nil {
			t.Fatal(err)
		}
		if _, err := Load(context.Background()); !errors.Is(err, ErrInvalidAppEnv) {
			t.Fatalf("error = %v", err)
		}
	})
}

func TestLoadLocalEnvironment(t *testing.T) {
	if err := loadLocalEnvironment(EnvTest); err != nil {
		t.Fatal(err)
	}
	if err := loadLocalEnvironment(EnvProduction); err != nil {
		t.Fatal(err)
	}

	t.Run("missing file", func(t *testing.T) {
		t.Chdir(t.TempDir())
		if err := loadLocalEnvironment(EnvDevelopment); err != nil {
			t.Fatal(err)
		}
	})
	t.Run("production file", func(t *testing.T) {
		t.Chdir(t.TempDir())
		unsetEnv(t, "APP_ENV")
		if err := os.WriteFile(".env", []byte("APP_ENV=production\n"), 0600); err != nil {
			t.Fatal(err)
		}
		if err := loadLocalEnvironment(EnvDevelopment); !errors.Is(err, ErrProductionEnvFile) {
			t.Fatalf("error = %v", err)
		}
	})
	t.Run("file error", func(t *testing.T) {
		t.Chdir(t.TempDir())
		if err := os.Mkdir(".env", 0700); err != nil {
			t.Fatal(err)
		}
		if err := loadLocalEnvironment(EnvDevelopment); !errors.Is(err, ErrEnvironmentFile) {
			t.Fatalf("error = %v", err)
		}
	})
}

func TestValidateConfig(t *testing.T) {
	valid := &Config{
		TelegramBotToken: "token", AdminTelegramID: 1, DatabasePath: "data/bot.db",
		AppTimezone: "UTC", ReminderHour: 9, InviteTTL: time.Hour,
		LogLevel: InfoLevel, AppEnv: EnvTest,
	}
	if err := validateConfig(valid); err != nil {
		t.Fatal(err)
	}
	invalid := *valid
	invalid.TelegramBotToken = ""
	invalid.AdminTelegramID = 0
	invalid.DatabasePath = "/tmp/bot.db"
	invalid.AppTimezone = "Invalid/Timezone"
	invalid.ReminderHour = 24
	invalid.InviteTTL = 0
	invalid.LogLevel = "trace"
	invalid.AppEnv = EnvProduction
	err := validateConfig(&invalid)
	for _, want := range []error{
		ErrTelegramBotTokenRequired, ErrAdminTelegramIDInvalid,
		ErrInvalidTimezone, ErrInvalidReminderHour, ErrInvalidInviteTTL,
		ErrInvalidLogLevel, ErrUnsafeProductionDatabase,
	} {
		if !errors.Is(err, want) {
			t.Errorf("missing %v in %v", want, err)
		}
	}

	missingDatabase := invalid
	missingDatabase.DatabasePath = ""
	if !errors.Is(validateConfig(&missingDatabase), ErrDatabasePathRequired) {
		t.Error("missing database path was not rejected")
	}
}

func TestReadConfigOptionalValueErrors(t *testing.T) {
	t.Run("invalid reminder hour", func(t *testing.T) {
		setBaseEnvironment(t, EnvTest)
		t.Setenv("REMINDER_HOUR", "bad")
		if _, err := readConfig(EnvTest); !errors.Is(err, ErrInvalidEnvironmentValue) {
			t.Fatalf("error = %v", err)
		}
	})
	t.Run("invalid invite ttl", func(t *testing.T) {
		setBaseEnvironment(t, EnvTest)
		t.Setenv("INVITE_TTL", "bad")
		if _, err := readConfig(EnvTest); !errors.Is(err, ErrInvalidEnvironmentValue) {
			t.Fatalf("error = %v", err)
		}
	})
}

func TestValidateTelegramBotToken(t *testing.T) {
	if !errors.Is(validateTelegramBotToken(context.Background(), ""), ErrTelegramBotTokenRequired) {
		t.Error("empty token was not rejected")
	}
	t.Run("request error", func(t *testing.T) {
		const token = "request-error-secret-token"

		old := telegramAPIBaseURL
		telegramAPIBaseURL = "://invalid"
		t.Cleanup(func() { telegramAPIBaseURL = old })
		err := validateTelegramBotToken(context.Background(), token)
		if !errors.Is(err, ErrCreatingTelegramRequest) {
			t.Error("request error was not returned")
		}
		if strings.Contains(err.Error(), token) {
			t.Errorf("request error leaked bot token: %v", err)
		}
	})
	t.Run("network error", func(t *testing.T) {
		const token = "network-error-secret-token"

		setTelegramRoundTripper(t, func(*http.Request) (*http.Response, error) {
			return nil, errors.New("network down")
		})
		err := validateTelegramBotToken(context.Background(), token)
		if !errors.Is(err, ErrTelegramAPIUnavailable) {
			t.Error("network error was not returned")
		}
		if strings.Contains(err.Error(), token) {
			t.Errorf("network error leaked bot token: %v", err)
		}
	})
	for _, tc := range []struct {
		name string
		code int
		body string
		want error
	}{
		{"decode", http.StatusOK, "not-json", ErrDecodeError},
		{"status unauthorized", http.StatusUnauthorized, "{\"ok\":false}", ErrTelegramBotTokenIsInvalid},
		{"code unauthorized", http.StatusOK, "{\"ok\":false,\"error_code\":401}", ErrTelegramBotTokenIsInvalid},
		{"http error", http.StatusBadGateway, "{\"ok\":false}", ErrTelegramAPIError},
		{"rejected", http.StatusOK, "{\"ok\":false,\"description\":\"bad token\"}", ErrTelegramRejectedBotToken},
	} {
		t.Run(tc.name, func(t *testing.T) {
			setTelegramResponse(t, tc.code, tc.body)
			if !errors.Is(validateTelegramBotToken(context.Background(), "token"), tc.want) {
				t.Errorf("unexpected error")
			}
		})
	}
	t.Run("success with nil context", func(t *testing.T) {
		setTelegramResponse(t, http.StatusOK, "{\"ok\":true}")
		if err := validateTelegramBotToken(context.TODO(), "token"); err != nil {
			t.Fatal(err)
		}
	})
}

func TestHelpers(t *testing.T) {
	clearConfigEnvironment(t)
	if optionalString("MISSING", "default") != "default" {
		t.Error("optionalString default failed")
	}
	t.Setenv("VALUE", " value ")
	if optionalString("VALUE", "default") != "value" {
		t.Error("optionalString value failed")
	}
	t.Setenv("INTEGER", "42")
	if got, err := requiredInt64("INTEGER"); err != nil || got != 42 {
		t.Errorf("requiredInt64 = %d, %v", got, err)
	}
	t.Setenv("INTEGER", "")
	if !errors.Is(func() error { _, err := requiredInt64("INTEGER"); return err }(), ErrRequiredEnvironmentVariable) {
		t.Error("requiredInt64 missing failed")
	}
	t.Setenv("OPTIONAL_INT", "")
	if got, _ := optionalInt("OPTIONAL_INT", 7); got != 7 {
		t.Error("optionalInt default failed")
	}
	t.Setenv("OPTIONAL_INT", "8")
	if got, _ := optionalInt("OPTIONAL_INT", 7); got != 8 {
		t.Error("optionalInt value failed")
	}
	t.Setenv("OPTIONAL_INT", "bad")
	if _, err := optionalInt("OPTIONAL_INT", 7); !errors.Is(err, ErrInvalidEnvironmentValue) {
		t.Error("optionalInt invalid failed")
	}
	t.Setenv("DURATION", "")
	if got, _ := optionalDuration("DURATION", time.Hour); got != time.Hour {
		t.Error("optionalDuration default failed")
	}
	t.Setenv("DURATION", "2h")
	if got, _ := optionalDuration("DURATION", time.Hour); got != 2*time.Hour {
		t.Error("optionalDuration value failed")
	}
	t.Setenv("DURATION", "bad")
	if _, err := optionalDuration("DURATION", time.Hour); !errors.Is(err, ErrInvalidEnvironmentValue) {
		t.Error("optionalDuration invalid failed")
	}
	for _, level := range []string{DebugLevel, InfoLevel, WarnLevel, ErrorLevel} {
		if !validLogLevel(level) {
			t.Errorf("level %q rejected", level)
		}
	}
	if validLogLevel("trace") {
		t.Error("trace accepted")
	}
}

func TestParseAppEnvAndDatabasePath(t *testing.T) {
	for _, tc := range []struct{ input, want string }{
		{"", EnvDevelopment}, {"dev", EnvDevelopment}, {" DEVELOPMENT ", EnvDevelopment},
		{"prod", EnvProduction}, {"production", EnvProduction}, {"test", EnvTest},
	} {
		got, err := parseAppEnv(tc.input)
		if err != nil || got != tc.want {
			t.Errorf("parseAppEnv(%q) = %q, %v", tc.input, got, err)
		}
	}
	if _, err := parseAppEnv("staging"); !errors.Is(err, ErrInvalidAppEnv) {
		t.Error("invalid APP_ENV accepted")
	}
	for _, path := range []string{":memory:", "C:/tmp/bot.db", "/var/tmp/bot.db"} {
		if !isUnsafeProductionDatabasePath(path) {
			t.Errorf("unsafe path accepted: %s", path)
		}
	}
	if isUnsafeProductionDatabasePath("data/bot.db") {
		t.Error("safe path rejected")
	}
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
}

func setTelegramResponse(t *testing.T, status int, body string) {
	t.Helper()
	setTelegramRoundTripper(t, func(*http.Request) (*http.Response, error) {
		return &http.Response{
			StatusCode: status, Body: io.NopCloser(strings.NewReader(body)),
			Header: make(http.Header),
		}, nil
	})
}

func setTelegramRoundTripper(t *testing.T, rt roundTripFunc) {
	t.Helper()
	oldURL, oldClient := telegramAPIBaseURL, telegramHTTPClient
	telegramAPIBaseURL = "https://telegram.test"
	telegramHTTPClient = &http.Client{Transport: rt}
	t.Cleanup(func() {
		telegramAPIBaseURL, telegramHTTPClient = oldURL, oldClient
	})
}

func setBaseEnvironment(t *testing.T, appEnv string) {
	t.Helper()
	clearConfigEnvironment(t)
	t.Setenv("APP_ENV", appEnv)
	t.Setenv("TELEGRAM_BOT_TOKEN", "test-token")
	t.Setenv("ADMIN_TELEGRAM_ID", "12345")
	t.Setenv("DATABASE_PATH", "./data/test.db")
}

func clearConfigEnvironment(t *testing.T) {
	t.Helper()
	for _, key := range []string{
		"APP_ENV", "TELEGRAM_BOT_TOKEN", "ADMIN_TELEGRAM_ID", "DATABASE_PATH",
		"APP_TIMEZONE", "REMINDER_HOUR", "INVITE_TTL", "LOG_LEVEL",
	} {
		t.Setenv(key, "")
	}
}

func unsetEnv(t *testing.T, key string) {
	t.Helper()
	value, exists := os.LookupEnv(key)
	_ = os.Unsetenv(key)
	t.Cleanup(func() {
		if exists {
			_ = os.Setenv(key, value)
		} else {
			_ = os.Unsetenv(key)
		}
	})
}
