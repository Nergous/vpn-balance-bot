# VPN Balance Bot

Telegram bot for tracking VPN account balances, payments, monthly charges, invitations, and debt reminders. It is designed for one administrator and pre-created user profiles.

## Current status

The repository contains a complete Go implementation with automated validation for configuration, localization, domain rules, SQLite persistence, Telegram handlers, billing, reminders, migrations, backup integrity, application composition, and a hardened systemd deployment template. A real-credential acceptance run remains environment-specific work.

The implementation sequence and acceptance criteria are maintained in [IMPLEMENTATION_PLAN.md](IMPLEMENTATION_PLAN.md). Linux installation, backup, restore, upgrade, rollback, monitoring, and acceptance procedures are documented in [docs/operations.md](docs/operations.md); the service template is [deploy/vpn-balance-bot.service](deploy/vpn-balance-bot.service).

## Implemented design

- Go version is declared in `go.mod`; CI reads that file instead of following a floating release.
- Application-owned sequential Telegram long polling; state-changing database work is durably deduplicated, while informational Telegram responses are sent only after the database transaction commits.
- SQLite through `database/sql` and `modernc.org/sqlite`, without CGO.
- Integer kopeks (`int64`) for money; no floating-point financial calculations.
- UTC timestamps and calendar calculations in `APP_TIMEZONE`.
- Immutable ledger entries with reversals instead of editing financial history.
- Additive migrations with unknown-version rejection and SHA-256 checksums for applied migration files.
- Durable payment drafts and leased/fenced reminder delivery attempts with explicit reconciliation for ambiguous sends.
- Embedded localization catalogs with Russian as the explicit default and English as a supported alternative.

## Configuration

For local development, copy `.env.example` to `.env` and replace placeholder secrets. Never commit `.env`. Production must provide variables through the process environment because the application refuses to load a local `.env` when `APP_ENV=production`.

| Variable | Required | Default | Contract |
|---|---:|---|---|
| `APP_ENV` | no | `development` | `development`, `test`, or `production` |
| `TELEGRAM_BOT_TOKEN` | yes | none | Bot token from BotFather; non-test startup verifies it through `getMe` |
| `ADMIN_TELEGRAM_ID` | yes | none | Positive numeric Telegram ID of the only administrator |
| `DATABASE_PATH` | yes | none | SQLite database path; production rejects memory, temp, and test-like paths |
| `APP_TIMEZONE` | no | `Europe/Moscow` | Valid IANA timezone |
| `REMINDER_HOUR` | no | `9` | Integer from `0` through `23` |
| `INVITE_TTL` | no | `168h` | Go duration of at least one second |
| `LOG_LEVEL` | no | `INFO` | `DEBUG`, `INFO`, `WARN`, or `ERROR`; input is case-insensitive |
| `BOT_LANG` | no | `ru` | Russian (`ru`) by default; English (`en`) is supported |
| `HTTP_TIMEOUT` | no | `30s` | Outbound HTTP timeout of at least `2s`; long polling always waits at least one second |
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

## Operations

- Never copy a live WAL database file directly. Use the backup command, which creates a SQLite snapshot, runs both `integrity_check` and `foreign_key_check`, applies restrictive file permissions, and publishes the destination with atomic no-replace semantics:

```bash
./vpn-balance-bot backup /var/backups/vpn-balance-bot/backup-2026-09-03.db
```

The command reads only `APP_ENV`, `DATABASE_PATH`, `DB_TIMEOUT`, and `LOG_LEVEL`; it requires the source database to exist, refuses an existing destination, and does not require or validate Telegram credentials.

- Startup rejects databases containing migration versions unknown to the running binary or checksums that do not match the embedded migration history.
- Startup recovers every reminder left pending by the previous single process as `delivery_state_unknown`. During runtime, expired reminder leases receive the same classification.
- After independently confirming that an ambiguous reminder was not delivered, the administrator can reopen it with `/admin reconcile <user-id> <YYYY-MM-DD> <reminder-type>`.
- Payment drafts survive process restarts and are deleted only after the ledger write succeeds or the administrator cancels them.

## Test isolation

Tests must use temporary SQLite databases created under `t.TempDir()` or in-memory fakes where appropriate. Telegram behavior is exercised through fake clients or local HTTP transports. Tests must not read production `.env`, open the live SQLite database, or contact the real Telegram API or other external services.

The focused configuration and localization tests do not open any database or contact the real Telegram API.

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

- A systemd unit and runbook are provided, but container packaging and automated deployment are not.
- Real Telegram credentials and network calls are intentionally excluded from automated tests.
- Production startup and restore drills still require environment-specific acceptance with a dedicated test bot and isolated database before real data is used.
- SQLite WAL databases require the verified snapshot procedure described above; copying only the main database file is not safe.

Do not treat a green unit-test run as production approval. Complete the remaining plan stages in an isolated test environment before deploying with real credentials or data.

## License

Released under the [MIT License](LICENSE).
