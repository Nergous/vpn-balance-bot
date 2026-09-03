package config

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/Nergous/vpn-balance-bot/internal/localization"
	"github.com/joho/godotenv"
)

const (
	EnvDevelopment = "development"
	EnvProduction  = "production"
	EnvTest        = "test"
)

const (
	InfoLevel  = "INFO"
	DebugLevel = "DEBUG"
	WarnLevel  = "WARN"
	ErrorLevel = "ERROR"
)

const defaultTimezone = "Europe/Moscow"
const minHTTPTimeout = 2 * time.Second

const (
	BotLanguageRussian = localization.Russian
	BotLanguageEnglish = localization.English
	defaultBotLanguage = localization.Default
)

var (
	telegramAPIBaseURL = "https://api.telegram.org"
	telegramHTTPClient = &http.Client{Timeout: 5 * time.Second}
)

type Config struct {
	TelegramBotToken string
	AdminTelegramID  int64
	DatabasePath     string
	AppTimezone      string
	ReminderHour     int
	InviteTTL        time.Duration
	LogLevel         string
	AppEnv           string
	BotLanguage      localization.Language
	HTTPTimeout      time.Duration
	DBTimeout        time.Duration
}

// Load reads and validates configuration. It does not contact Telegram in test mode.
func Load(ctx context.Context) (*Config, error) {
	return load(ctx, true)
}

// LoadForMaintenance reads only database and logging settings. It does not
// require Telegram credentials or contact external services.
func LoadForMaintenance(_ context.Context) (*Config, error) {
	return loadMaintenanceConfig("", true)
}

// LoadForInspection reads logging and database timeout settings for an explicit
// database path. It does not require Telegram credentials or contact services.
func LoadForInspection(_ context.Context, databasePath string) (*Config, error) {
	return loadMaintenanceConfig(databasePath, false)
}

func loadMaintenanceConfig(databasePath string, enforceProductionPath bool) (*Config, error) {
	appEnv, err := parseAppEnv(os.Getenv("APP_ENV"))
	if err != nil {
		return nil, err
	}
	if err := loadLocalEnvironment(appEnv); err != nil {
		return nil, err
	}
	appEnv, err = parseAppEnv(os.Getenv("APP_ENV"))
	if err != nil {
		return nil, err
	}
	dbTimeout, err := optionalDuration("DB_TIMEOUT", 30*time.Second)
	if err != nil {
		return nil, err
	}
	cfg := &Config{
		DatabasePath: strings.TrimSpace(databasePath),
		DBTimeout:    dbTimeout,
		LogLevel:     strings.ToUpper(optionalString("LOG_LEVEL", InfoLevel)),
		AppEnv:       appEnv,
	}
	if cfg.DatabasePath == "" {
		cfg.DatabasePath = strings.TrimSpace(os.Getenv("DATABASE_PATH"))
	}
	if err := validateMaintenanceConfig(cfg, enforceProductionPath); err != nil {
		return nil, err
	}
	return cfg, nil
}

func load(ctx context.Context, validateToken bool) (*Config, error) {
	appEnv, err := parseAppEnv(os.Getenv("APP_ENV"))
	if err != nil {
		return nil, err
	}
	if err := loadLocalEnvironment(appEnv); err != nil {
		return nil, err
	}

	appEnv, err = parseAppEnv(os.Getenv("APP_ENV"))
	if err != nil {
		return nil, err
	}
	cfg, err := readConfig(appEnv)
	if err != nil {
		return nil, err
	}

	if err := validateConfig(cfg); err != nil {
		return nil, err
	}
	if validateToken && cfg.AppEnv != EnvTest {
		if err := validateTelegramBotToken(ctx, cfg.TelegramBotToken); err != nil {
			return nil, err
		}
	}

	return cfg, nil
}

func validateMaintenanceConfig(cfg *Config, enforceProductionPath bool) error {
	var errs []error
	if cfg.DatabasePath == "" {
		errs = append(errs, ErrDatabasePathRequired)
	}
	if cfg.DBTimeout <= 0 {
		errs = append(errs, ErrInvalidDBTimeout)
	}
	if !validLogLevel(cfg.LogLevel) {
		errs = append(errs, fmt.Errorf("%w: %q", ErrInvalidLogLevel, cfg.LogLevel))
	}
	if enforceProductionPath && cfg.AppEnv == EnvProduction &&
		isUnsafeProductionDatabasePath(cfg.DatabasePath) {
		errs = append(errs, ErrUnsafeProductionDatabase)
	}
	return errors.Join(errs...)
}

// MustLoad is intended for application startup.
func MustLoad(ctx context.Context) *Config {
	cfg, err := Load(ctx)
	if err != nil {
		panic(err)
	}
	return cfg
}

func loadLocalEnvironment(appEnv string) error {
	switch appEnv {
	case EnvTest, EnvProduction:
		return nil
	}

	if err := godotenv.Load(".env"); err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("%w: %w", ErrEnvironmentFile, err)
	}

	env, err := parseAppEnv(os.Getenv("APP_ENV"))
	if err == nil && env == EnvProduction {
		return ErrProductionEnvFile
	}
	return nil
}

func readConfig(appEnv string) (*Config, error) {
	adminID, err := requiredInt64("ADMIN_TELEGRAM_ID")
	if err != nil {
		return nil, err
	}

	reminderHour, err := optionalInt("REMINDER_HOUR", 9)
	if err != nil {
		return nil, err
	}

	inviteTTL, err := optionalDuration("INVITE_TTL", 168*time.Hour)
	if err != nil {
		return nil, err
	}

	httpTimeout, err := optionalDuration("HTTP_TIMEOUT", 30*time.Second)
	if err != nil {
		return nil, err
	}

	dbTimeout, err := optionalDuration("DB_TIMEOUT", 30*time.Second)
	if err != nil {
		return nil, err
	}
	botLanguage, err := localization.Parse(optionalString("BOT_LANG", string(defaultBotLanguage)))
	if err != nil {
		return nil, fmt.Errorf("%w: %w", ErrInvalidBotLanguage, err)
	}

	return &Config{
		TelegramBotToken: strings.TrimSpace(os.Getenv("TELEGRAM_BOT_TOKEN")),
		AdminTelegramID:  adminID,
		DatabasePath:     strings.TrimSpace(os.Getenv("DATABASE_PATH")),
		AppTimezone:      optionalString("APP_TIMEZONE", defaultTimezone),
		ReminderHour:     reminderHour,
		InviteTTL:        inviteTTL,
		LogLevel:         strings.ToUpper(optionalString("LOG_LEVEL", InfoLevel)),
		AppEnv:           appEnv,
		BotLanguage:      botLanguage,
		HTTPTimeout:      httpTimeout,
		DBTimeout:        dbTimeout,
	}, nil
}

func validateConfig(cfg *Config) error {
	var errs []error

	if cfg.TelegramBotToken == "" {
		errs = append(errs, ErrTelegramBotTokenRequired)
	}
	if cfg.AdminTelegramID <= 0 {
		errs = append(errs, ErrAdminTelegramIDInvalid)
	}
	if cfg.DatabasePath == "" {
		errs = append(errs, ErrDatabasePathRequired)
	}
	if _, err := time.LoadLocation(cfg.AppTimezone); err != nil {
		errs = append(errs, fmt.Errorf("%w: %q: %w", ErrInvalidTimezone, cfg.AppTimezone, err))
	}
	if cfg.ReminderHour < 0 || cfg.ReminderHour > 23 {
		errs = append(errs, ErrInvalidReminderHour)
	}
	if cfg.InviteTTL < time.Second {
		errs = append(errs, ErrInvalidInviteTTL)
	}
	if cfg.HTTPTimeout < minHTTPTimeout {
		errs = append(errs, ErrInvalidHTTPTimeout)
	}
	if cfg.DBTimeout <= 0 {
		errs = append(errs, ErrInvalidDBTimeout)
	}
	if !validLogLevel(cfg.LogLevel) {
		errs = append(errs, fmt.Errorf("%w: %q", ErrInvalidLogLevel, cfg.LogLevel))
	}
	if !validBotLanguage(cfg.BotLanguage) {
		errs = append(errs, ErrInvalidBotLanguage)
	}
	if cfg.AppEnv == EnvProduction && isUnsafeProductionDatabasePath(cfg.DatabasePath) {
		errs = append(errs, ErrUnsafeProductionDatabase)
	}

	return errors.Join(errs...)
}

func validateTelegramBotToken(ctx context.Context, token string) error {
	if token == "" {
		return ErrTelegramBotTokenRequired
	}

	if ctx == nil {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
	}

	endpoint := strings.TrimRight(telegramAPIBaseURL, "/") + "/bot" + url.PathEscape(token) + "/getMe"
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return ErrCreatingTelegramRequest
	}

	resp, err := telegramHTTPClient.Do(req)
	if err != nil {
		return ErrTelegramAPIUnavailable
	}
	defer resp.Body.Close()

	var payload map[string]json.RawMessage
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return fmt.Errorf("%w: %w", ErrDecodeError, err)
	}

	var ok bool
	_ = json.Unmarshal(payload["ok"], &ok)

	var errorCode int
	_ = json.Unmarshal(payload["error_code"], &errorCode)

	var description string
	_ = json.Unmarshal(payload["description"], &description)

	if resp.StatusCode == http.StatusUnauthorized || errorCode == http.StatusUnauthorized {
		return ErrTelegramBotTokenIsInvalid
	}
	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		return fmt.Errorf("%w: HTTP %d", ErrTelegramAPIError, resp.StatusCode)
	}
	if !ok {
		return fmt.Errorf("%w: %s", ErrTelegramRejectedBotToken, description)
	}

	return nil
}

func parseAppEnv(value string) (string, error) {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "", "dev", EnvDevelopment:
		return EnvDevelopment, nil
	case "prod", EnvProduction:
		return EnvProduction, nil
	case EnvTest:
		return EnvTest, nil
	default:
		return "", fmt.Errorf("%w: %q (expected %q, %q, or %q)", ErrInvalidAppEnv, value, EnvDevelopment, EnvProduction, EnvTest)
	}
}

func optionalString(key, defaultValue string) string {
	if value := strings.TrimSpace(os.Getenv(key)); value != "" {
		return value
	}
	return defaultValue
}

func requiredInt64(key string) (int64, error) {
	value := strings.TrimSpace(os.Getenv(key))
	if value == "" {
		return 0, fmt.Errorf("%w: %s", ErrRequiredEnvironmentVariable, key)
	}

	parsed, err := strconv.ParseInt(value, 10, 64)
	if err != nil {
		return 0, fmt.Errorf("%w: %s: %w", ErrInvalidEnvironmentValue, key, err)
	}
	return parsed, nil
}

func optionalInt(key string, defaultValue int) (int, error) {
	value := strings.TrimSpace(os.Getenv(key))
	if value == "" {
		return defaultValue, nil
	}

	parsed, err := strconv.Atoi(value)
	if err != nil {
		return 0, fmt.Errorf("%w: %s: %w", ErrInvalidEnvironmentValue, key, err)
	}
	return parsed, nil
}

func optionalDuration(key string, defaultValue time.Duration) (time.Duration, error) {
	value := strings.TrimSpace(os.Getenv(key))
	if value == "" {
		return defaultValue, nil
	}

	parsed, err := time.ParseDuration(value)
	if err != nil {
		return 0, fmt.Errorf("%w: %s: %w", ErrInvalidEnvironmentValue, key, err)
	}
	return parsed, nil
}

func validLogLevel(value string) bool {
	switch value {
	case DebugLevel, InfoLevel, WarnLevel, ErrorLevel:
		return true
	default:
		return false
	}
}

func validBotLanguage(value localization.Language) bool {
	return value.IsSupported()
}

func isUnsafeProductionDatabasePath(path string) bool {
	lower := strings.ToLower(strings.TrimSpace(path))
	return lower == ":memory:" ||
		strings.Contains(lower, "tmp") ||
		strings.Contains(lower, "temp") ||
		strings.Contains(lower, "test")
}
