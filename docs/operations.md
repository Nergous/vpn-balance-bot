# Production operations

VPN Balance Bot supports two single-host deployment models:

- Docker Compose with a named SQLite volume.
- A native Linux binary managed by systemd.

Both models must run exactly one bot process against one database. Do not mount
the same SQLite database into multiple running application containers or services.

Before using production credentials, complete the acceptance checklist with a
dedicated test bot and an isolated database.

## Docker Compose

Create the configuration:

```bash
git clone https://github.com/Nergous/vpn-balance-bot.git
cd vpn-balance-bot
cp .env.example .env
chmod 600 .env
```

Set `TELEGRAM_BOT_TOKEN` and `ADMIN_TELEGRAM_ID`. Compose overrides
`APP_ENV` and `DATABASE_PATH` so the container always uses production config
and the `/data` volume.

Build and start:

```bash
docker compose config
docker compose build
docker compose up -d
docker compose ps
docker compose logs --tail 100 bot
```

After startup, `docker compose ps` should report the service as healthy. The
healthcheck runs `vpn-balance-bot doctor` against the existing database without
contacting Telegram or applying migrations.

The service publishes no ports. It only makes outbound HTTPS requests to the
Telegram Bot API.

Stop without deleting the database:

```bash
docker compose down
```

Never add `--volumes` to this command unless permanent database deletion is
explicitly intended and a verified external backup exists.

## Native systemd service

Build and verify the binary:

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

Create `/etc/vpn-balance-bot/vpn-balance-bot.env` with owner
`root:vpn-balance-bot` and mode `0640`:

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

Do not quote systemd environment-file values. Keep the bot token out of shell
history, source control, command lines, and support logs.

Validate and start:

```bash
sudo systemd-analyze verify /etc/systemd/system/vpn-balance-bot.service
sudo systemctl daemon-reload
sudo systemctl enable --now vpn-balance-bot.service
sudo systemctl status vpn-balance-bot.service
sudo journalctl -u vpn-balance-bot.service -n 100 --no-pager
```

Run a local database health check under the service account:

```bash
sudo -u vpn-balance-bot /usr/bin/env \
  APP_ENV=production \
  DATABASE_PATH=/var/lib/vpn-balance-bot/vpn-balance-bot.db \
  DB_TIMEOUT=30s \
  LOG_LEVEL=info \
  /opt/vpn-balance-bot/vpn-balance-bot doctor
```

The unit runs as an unprivileged account with a strict filesystem sandbox,
private temporary directory, no Linux capabilities, and write access only to its
state directory.

## Acceptance checklist

Use a dedicated test bot and a fresh non-production database:

1. Startup reaches running state and logs `application started`.
2. A non-admin cannot use any `/admin` command.
3. The administrator can create a customer and a single-use invitation.
4. The invited account can use `/status` and `/history`.
5. A payment remains a draft until explicit confirmation.
6. Restarting does not duplicate the confirmed payment.
7. Automatic and manual reminders do not duplicate after restart.
8. A backup can replace a deleted test database and restart successfully.
9. The previous binary and pre-upgrade backup complete a rollback rehearsal.

Only after the checklist passes should production credentials and data be used.

## Upgrade

Always create an external verified backup first.

Docker Compose:

```bash
docker compose build --pull
docker compose up -d
docker compose ps
docker compose logs --tail 100 bot
```

systemd:

```bash
sudo systemctl stop vpn-balance-bot.service
sudo cp --preserve=mode,ownership \
  /opt/vpn-balance-bot/vpn-balance-bot \
  /opt/vpn-balance-bot/vpn-balance-bot.previous
sudo install -o root -g root -m 0755 \
  vpn-balance-bot \
  /opt/vpn-balance-bot/vpn-balance-bot
sudo systemctl start vpn-balance-bot.service
sudo systemctl status vpn-balance-bot.service
```

Startup validates the token, applies embedded migrations, and rejects unknown or
checksum-mismatched migration history.

## Rollback

If no new migration was applied, return to the previous image or binary and
restart. If a new migration was applied, restore both:

1. The previous image or binary.
2. The verified pre-upgrade database snapshot.

Do not edit `schema_migrations` or migration checksum rows manually.

## Monitoring

Docker:

```bash
docker compose ps
docker compose logs --since 1h bot
docker inspect --format '{{.RestartCount}}' "$(docker compose ps -q bot)"
```

systemd:

```bash
sudo systemctl is-active vpn-balance-bot.service
sudo systemctl show vpn-balance-bot.service -p NRestarts
sudo journalctl -u vpn-balance-bot.service --since "1 hour ago" --no-pager
```

Alert on repeated restarts, migration failures, backup failures,
`scheduler run failed`, and growth in unreachable recipients.

## Related runbooks

- [Backup and restore](backup-and-restore.md)
- [Troubleshooting](troubleshooting.md)
- [Database design](database.md)
