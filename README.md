# VPN Balance Bot

Telegram bot for tracking VPN account balances, payments, monthly charges, invitations, and debt reminders. It is designed for one administrator and pre-created user profiles.

## Current status

The repository contains an active Go implementation: configuration, localization, domain types, services, SQLite storage, Telegram handlers, migrations, and application composition are present. The project is still under implementation and is not yet verified as production-ready.

The implementation sequence, acceptance criteria, and planned deployment work are maintained in [IMPLEMENTATION_PLAN.md](IMPLEMENTATION_PLAN.md). No `deploy/systemd` unit or complete production deployment runbook exists in the current repository.

## Implemented design

- Go version is declared in `go.mod`; CI reads that file instead of following a floating release.
- Telegram long polling through `github.com/go-telegram/bot`.
- SQLite through `database/sql` and `modernc.org/sqlite`, without CGO.
- Integer kopeks (`int64`) for money; no floating-point financial calculations.
- UTC timestamps and calendar calculations in `APP_TIMEZONE`.
- Immutable ledger entries with reversals instead of editing financial history.
- Embedded localization catalogs with Russian as the explicit default and English as a supported alternative.

## Configuration

For local development, copy `.env.example` to `.env` and replace placeholder secrets. Never commit `.env`. Production must provide variables through the process environment because the application refuses to load a local `.env` when `APP_ENV=production`.

| Variable | Required | Default | Contract |
|---|---:|---|---|
| `APP_ENV` | no | `development` | `development`, `test`, or `production` |
| `TELEGRAM_BOT_TOKEN` | yes | none | Bot token from BotFather |
| `ADMIN_TELEGRAM_ID` | yes | none | Positive numeric Telegram ID of the only administrator |
| `DATABASE_PATH` | yes | none | SQLite database path; production rejects memory, temp, and test-like paths |
| `APP_TIMEZONE` | no | `Europe/Moscow` | Valid IANA timezone |
| `REMINDER_HOUR` | no | `9` | Integer from `0` through `23` |
| `INVITE_TTL` | no | `168h` | Positive Go duration |
| `LOG_LEVEL` | no | `INFO` | `DEBUG`, `INFO`, `WARN`, or `ERROR`; input is case-insensitive |
| `BOT_LANG` | no | `ru` | Russian (`ru`) by default; English (`en`) is supported |
| `HTTP_TIMEOUT` | no | `30s` | Positive Go duration for outbound HTTP operations |
| `DB_TIMEOUT` | no | `30s` | Positive Go duration for database operations |

## Checks

Focused checks for configuration and localization:

```bash
go test ./internal/config ./internal/localization
go mod tidy -diff
gofmt -l .
```

Broader repository checks:

```bash
go vet ./...
go test ./...
go build ./cmd/vpn-balance-bot
```

Linux CI also runs `go test -race ./...`.

## Test isolation

Tests must use temporary SQLite databases created under `t.TempDir()` or in-memory fakes where appropriate. Telegram behavior is exercised through fake clients or local HTTP transports. Tests must not read production `.env`, open the live SQLite database, or contact the real Telegram API or other external services.

The focused configuration and localization tests do not open any database or contact Telegram.

## Repository structure

```text
cmd/vpn-balance-bot/       application entry point
internal/app/              composition and lifecycle
internal/config/           environment loading and validation
internal/domain/           business types and rules
internal/localization/     embedded Russian and English catalogs
internal/service/          account, billing, reminder, and scheduler use cases
internal/storage/sqlite/   SQLite implementation and backup logic
internal/telegram/         Telegram adapter and handlers
internal/testutil/         test fakes and helpers
migrations/                embedded SQL migrations
```

## Production limitations

- No systemd unit, container image, packaging workflow, or automated deployment is currently provided.
- Real Telegram credentials and network calls are intentionally excluded from automated tests.
- Production startup, backup/restore, permissions, monitoring, restart behavior, and rollback still require the acceptance work described in `IMPLEMENTATION_PLAN.md`.
- SQLite WAL databases require a consistent SQLite backup procedure; copying only the main database file is not a safe backup strategy.

Do not treat a green unit-test run as production approval. Complete the remaining plan stages in an isolated test environment before deploying with real credentials or data.

## License

Released under the [MIT License](LICENSE).
