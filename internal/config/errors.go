package config

import "errors"

var (
	ErrRequiredEnvironmentVariable = errors.New("required environment variable is missing")
	ErrInvalidEnvironmentValue     = errors.New("environment variable has an invalid value")
	ErrInvalidAppEnv               = errors.New("APP_ENV is invalid")
	ErrEnvironmentFile             = errors.New("failed to load environment file")
	ErrProductionEnvFile           = errors.New("production must not load a local environment file")
	ErrAdminTelegramIDInvalid      = errors.New("admin Telegram ID must be positive")
	ErrDatabasePathRequired        = errors.New("database path is required")
	ErrInvalidTimezone             = errors.New("timezone is invalid")
	ErrInvalidReminderHour         = errors.New("reminder hour is invalid")
	ErrInvalidInviteTTL            = errors.New("invite TTL is invalid")
	ErrInvalidHTTPTimeout          = errors.New("HTTP_TIMEOUT must be positive")
	ErrInvalidDBTimeout            = errors.New("DB_TIMEOUT must be positive")
	ErrInvalidLogLevel             = errors.New("log level is invalid")
	ErrInvalidBotLanguage          = errors.New("bot language is unsupported")
	ErrUnsafeProductionDatabase    = errors.New("production database path is unsafe")

	ErrTelegramBotTokenRequired = errors.New("telegram bot token is required")

	ErrCreatingTelegramRequest   = errors.New("failed to create telegram request")
	ErrTelegramAPIUnavailable    = errors.New("telegram api is unavailable")
	ErrTelegramBotTokenIsInvalid = errors.New("telegram bot token is invalid")
	ErrTelegramAPIError          = errors.New("telegram api error")
	ErrDecodeError               = errors.New("failed to decode response")
	ErrTelegramRejectedBotToken  = errors.New("telegram rejected bot token")
)
