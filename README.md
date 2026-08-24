# VPN Balance Bot

Telegram bot for tracking VPN balances, payments, monthly charges, and reminders.

The project is designed for one administrator and users whose profiles are created in advance and who receive one-time Telegram invitation links.

## Features

- users can view their balance, plan, next charge date, and recent operations;
- administrators can create users and record actual payments;
- ledger model with payments, charges, adjustments, and `reversal` entries;
- automatic monthly charges with catch-up after downtime;
- reminders for insufficient balance and outstanding debt;
- protection against duplicate charges and reminder deliveries;
- SQLite without CGO, one process, and Telegram long polling.

## Stack

- Go — the latest stable version;
- [`github.com/go-telegram/bot`](https://github.com/go-telegram/bot);
- `database/sql`;
- `modernc.org/sqlite`;
- `log/slog`;
- embedded SQL migrations through `go:embed`.

## Status

Requirements and implementation stages are described in [PROJECT_PLAN.md](PROJECT_PLAN.md). Source code and `go.mod` will be added according to the plan.

## Configuration

Copy `.env.example` to `.env` and fill in the values. Never commit `.env` to git.

Main variables:

| Variable | Required | Purpose |
|---|---:|---|
| `TELEGRAM_BOT_TOKEN` | yes | bot token from BotFather |
| `ADMIN_TELEGRAM_ID` | yes | administrator's numeric Telegram ID |
| `DATABASE_PATH` | yes | path to the SQLite database |
| `APP_TIMEZONE` | no | timezone, defaults to `Europe/Moscow` |
| `REMINDER_HOUR` | no | reminder hour, defaults to `9` |
| `INVITE_TTL` | no | invite token lifetime, defaults to `168h` |
| `LOG_LEVEL` | no | log level, defaults to `info` |

## Checks

```bash
go vet ./...
go test ./...
go build ./cmd/vpn-balance-bot
```

Linux CI also runs `go test -race ./...`.

Tests use only SQLite in `t.TempDir()` and a fake Telegram client. Tests never use the production `.env`, production database, or the real Telegram API.

## Project structure

```text
cmd/vpn-balance-bot/main.go
internal/app/
internal/config/
internal/domain/
internal/service/account/
internal/service/reminder/
internal/storage/sqlite/
internal/telegram/
internal/testutil/
migrations/
deploy/systemd/
```

## Data and production

Money values are stored as integer kopeks (`int64`), timestamps use UTC, and billing dates use `APP_TIMEZONE`.

The live SQLite database uses WAL. Do not copy only the main database file with a regular file command; use a consistent SQLite backup procedure. Before a production update, create a backup and verify the restore against a separate test database.

The target deployment is a Linux VPS with systemd: the binary in `/opt/vpn-balance-bot/`, data in `/var/lib/vpn-balance-bot/`, and secrets in `/etc/vpn-balance-bot.env` with permissions `0600`.

Detailed requirements are documented in [PROJECT_PLAN.md](PROJECT_PLAN.md).

## License

This project is released under the [MIT](LICENSE) license.
