# Production operations

This runbook targets a single Linux host managed by systemd. The service must run as a dedicated unprivileged account and must be the only process writing the SQLite database.

## Install

Build and verify the binary before copying it to the host:

```bash
go test ./...
go test -race ./...
go vet ./...
go build -trimpath -o vpn-balance-bot ./cmd/vpn-balance-bot
```

Create the runtime account and protected directories:

```bash
sudo useradd --system --home /var/lib/vpn-balance-bot --shell /usr/sbin/nologin vpn-balance-bot
sudo install -d -o root -g root -m 0755 /opt/vpn-balance-bot
sudo install -d -o root -g vpn-balance-bot -m 0750 /etc/vpn-balance-bot
sudo install -d -o vpn-balance-bot -g vpn-balance-bot -m 0700 /var/lib/vpn-balance-bot
sudo install -o root -g root -m 0755 vpn-balance-bot /opt/vpn-balance-bot/vpn-balance-bot
sudo install -o root -g root -m 0644 deploy/vpn-balance-bot.service /etc/systemd/system/vpn-balance-bot.service
```

Create `/etc/vpn-balance-bot/vpn-balance-bot.env` with mode `0640`, owner `root`, and group `vpn-balance-bot`:

```dotenv
APP_ENV=production
TELEGRAM_BOT_TOKEN=replace-with-botfather-token
ADMIN_TELEGRAM_ID=123456789
DATABASE_PATH=/var/lib/vpn-balance-bot/vpn-balance-bot.db
APP_TIMEZONE=Europe/Moscow
REMINDER_HOUR=9
INVITE_TTL=168h
LOG_LEVEL=info
BOT_LANG=ru
HTTP_TIMEOUT=30s
DB_TIMEOUT=30s
```

Do not place quotes around systemd environment-file values. Keep the token out of shell history, source control, command lines, and support logs.

Validate and start:

```bash
sudo systemd-analyze verify /etc/systemd/system/vpn-balance-bot.service
sudo systemctl daemon-reload
sudo systemctl enable --now vpn-balance-bot.service
sudo systemctl status vpn-balance-bot.service
sudo journalctl -u vpn-balance-bot.service -n 100 --no-pager
```

Startup validates the Telegram token, applies embedded migrations, rejects unknown or checksum-mismatched migrations, and recovers interrupted reminder deliveries as ambiguous rather than resending them automatically.

## Acceptance check

Use a dedicated test bot and a fresh non-production database for the first host-level rehearsal. Verify:

1. The service reaches `active (running)` and logs `application started`.
2. A non-admin cannot use `/admin` commands.
3. The configured administrator can create a user and invitation.
4. The invited account can use `/status` and `/history`.
5. A payment confirmation shows user, amount, UTC date, comment, and explicit confirm/cancel commands.
6. `/admin` shows debtors, insufficient balances, unlinked profiles, and unreachable recipients.
7. Restarting the service does not duplicate a confirmed payment or reminder.

Only after this rehearsal should the production token and production database path be configured.

## Backup

Never copy the live `.db`, `-wal`, or `-shm` files. The built-in command creates a consistent snapshot, validates SQLite integrity and foreign keys, refuses to overwrite an existing destination, and publishes with restrictive permissions.

```bash
sudo install -d -o vpn-balance-bot -g vpn-balance-bot -m 0700 /var/backups/vpn-balance-bot
sudo -u vpn-balance-bot /usr/bin/env \
  APP_ENV=production \
  DATABASE_PATH=/var/lib/vpn-balance-bot/vpn-balance-bot.db \
  DB_TIMEOUT=30s \
  LOG_LEVEL=info \
  /opt/vpn-balance-bot/vpn-balance-bot backup \
  /var/backups/vpn-balance-bot/backup-2026-09-03.db
```

Copy completed snapshots to storage outside the host and periodically perform a restore rehearsal.

## Upgrade and rollback

Before every upgrade, create a verified backup. Keep the previously deployed binary until acceptance succeeds.

```bash
sudo systemctl stop vpn-balance-bot.service
sudo cp --preserve=mode,ownership /opt/vpn-balance-bot/vpn-balance-bot /opt/vpn-balance-bot/vpn-balance-bot.previous
sudo install -o root -g root -m 0755 vpn-balance-bot /opt/vpn-balance-bot/vpn-balance-bot
sudo systemctl start vpn-balance-bot.service
sudo systemctl status vpn-balance-bot.service
```

If startup or acceptance fails, stop the service, restore `vpn-balance-bot.previous`, and start it again. If the new binary applied a migration that the previous binary does not recognize, restore the pre-upgrade database snapshot as described below; do not edit migration tables manually.

## Restore rehearsal or recovery

1. Stop the service and confirm no process has the database open.
2. Preserve the current database directory for forensic recovery.
3. Install a verified snapshot as `/var/lib/vpn-balance-bot/vpn-balance-bot.db` with owner `vpn-balance-bot:vpn-balance-bot` and mode `0600`.
4. Remove stale `-wal` and `-shm` files only from the stopped service's database directory.
5. Start the service and inspect startup logs. Migration checksum failure or an unknown migration means the snapshot and binary do not belong to the same release history.
6. Repeat the acceptance check before reopening normal administration.

## Monitoring and incident response

Useful commands:

```bash
sudo systemctl is-active vpn-balance-bot.service
sudo journalctl -u vpn-balance-bot.service --since "1 hour ago" --no-pager
sudo systemctl restart vpn-balance-bot.service
```

Alert on repeated restarts, `scheduler run failed`, migration failures, backup failures, and growth in the dashboard's unreachable count. Handler logs intentionally contain correlation identifiers and error classes, not Telegram message text, tokens, comments, or filesystem paths.

For an ambiguous reminder delivery, first verify independently that Telegram did not deliver it, then use `/admin reconcile <user-id> <YYYY-MM-DD> <reminder-type>`. Do not reconcile merely to silence an error.
