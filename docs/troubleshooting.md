# Troubleshooting

Start with the service status and recent structured logs. Do not paste bot tokens,
invite tokens, customer comments, database files, or full environment files into
issues or support messages.

## Docker service

```bash
docker compose config
docker compose ps
docker compose logs --tail 200 bot
docker inspect --format '{{json .State}}' "$(docker compose ps -q bot)"
```

If `docker compose config` reports that `.env` is missing, copy
`.env.example` to `.env`, set the required values, and keep mode `0600` on
Linux.

If the container restarts immediately, inspect logs before running it again.
Typical causes are invalid configuration, rejected Telegram credentials, an
unsafe production database path, or migration incompatibility.

Run the offline database checks directly:

```bash
docker compose exec bot /usr/local/bin/vpn-balance-bot doctor
docker compose exec bot /usr/local/bin/vpn-balance-bot migrate-status
```

## systemd service

```bash
sudo systemctl status vpn-balance-bot.service
sudo systemctl show vpn-balance-bot.service -p NRestarts -p ExecMainStatus
sudo journalctl -u vpn-balance-bot.service -n 200 --no-pager
sudo systemd-analyze verify /etc/systemd/system/vpn-balance-bot.service
```

After editing the unit:

```bash
sudo systemctl daemon-reload
sudo systemctl restart vpn-balance-bot.service
```

## Configuration rejected

Check:

- `TELEGRAM_BOT_TOKEN` is present and was issued by BotFather.
- `ADMIN_TELEGRAM_ID` is a positive numeric user ID.
- `DATABASE_PATH` is absolute in production and is not a temp or test path.
- `APP_TIMEZONE` is a valid IANA timezone.
- `REMINDER_HOUR` is between `0` and `23`.
- `HTTP_TIMEOUT` is at least `2s`.
- `DB_TIMEOUT` is positive.
- `BOT_LANG` is `ru` or `en`.

Production does not load a local `.env`; systemd or Compose must inject the
variables into the process.

## Telegram API unavailable

Confirm the host can resolve and connect to `api.telegram.org:443`. Do not print
the token in diagnostic commands.

```bash
getent hosts api.telegram.org
curl --head --connect-timeout 5 https://api.telegram.org
```

An HTTP response confirms DNS and TLS connectivity; it does not validate the bot
token. Startup performs token validation through Telegram `getMe`.

## Database permission failure

Docker:

```bash
docker volume inspect vpn-balance-bot_vpn-balance-data
docker image inspect vpn-balance-bot:local --format '{{.Config.User}}'
```

The image must run as `65532:65532`. Do not solve permission errors by making
the database world-writable.

systemd:

```bash
sudo namei -l /var/lib/vpn-balance-bot/vpn-balance-bot.db
sudo ls -la /var/lib/vpn-balance-bot
```

The state directory should be owned by `vpn-balance-bot:vpn-balance-bot` with
mode `0700`; the database should use mode `0600`.

## Database is locked or busy

Only one bot process may write the database:

```bash
docker compose ps
sudo systemctl status vpn-balance-bot.service
```

Do not run the Compose and systemd deployments against the same database. Stop the
unexpected second process, then inspect logs. Do not delete WAL or SHM files while
any process has the database open.

## Unknown migration or checksum mismatch

Stop deployment. The database was opened by a binary from another migration
history, or an applied migration file was modified.

- Do not edit migration registry tables.
- Identify the application version that created the database.
- Restore the matching binary and verified snapshot.
- Preserve the rejected database for investigation.

## Reminder marked `delivery_state_unknown`

This state means the process cannot prove whether Telegram accepted the message.
Verify independently that the message was not delivered. Only then use:

```text
/admin reconcile <user-id> <YYYY-MM-DD> <reminder-type> [delivery-key]
```

Manual reminders require their exact `delivery-key`, such as `update:12345`.
Never reconcile solely to clear an error.

## Backup command failed

- The source database must already exist.
- The destination directory must be writable.
- The destination and its `.tmp` path must not already exist.
- Source and destination should be on filesystems that support hard links.
- Keep enough free space for another complete database snapshot.

The command intentionally refuses overwrite. Choose a new timestamped destination
instead of deleting an existing verified backup.

## Safe information for a bug report

Include:

- application version or commit;
- operating system and architecture;
- Docker or systemd deployment;
- stable `error_kind` and `error_type`;
- migration filenames, not database contents;
- minimal reproduction using a temporary database.

Exclude all secrets and customer data.
