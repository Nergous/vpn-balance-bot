# Backup and restore

VPN Balance Bot uses SQLite in WAL mode. Never back up a running service by
copying only `vpn-balance-bot.db`; committed pages may still be in the WAL file.

The built-in `backup` command uses SQLite `VACUUM INTO`, runs
`integrity_check` and `foreign_key_check`, sets mode `0600`, and refuses to
overwrite an existing destination.

## Backup with Docker Compose

Prepare a host directory writable by the container's numeric user:

```bash
mkdir -p backups
sudo chown 65532:65532 backups
chmod 700 backups
```

Create a timestamped snapshot through a one-off container. The main service may
remain running:

```bash
STAMP="$(date -u +%Y%m%dT%H%M%SZ)"
docker compose run --rm --no-deps \
  -v "$PWD/backups:/backups" \
  bot backup "/backups/vpn-balance-bot-$STAMP.db"
sudo chown "$(id -u):$(id -g)" "backups/vpn-balance-bot-$STAMP.db"
sha256sum "backups/vpn-balance-bot-$STAMP.db" \
  > "backups/vpn-balance-bot-$STAMP.db.sha256"
```

Copy the snapshot and checksum to storage outside the Docker host.

Verify any copied snapshot without the live database:

```bash
docker compose run --rm --no-deps \
  -v "$PWD/backups:/backups:ro" \
  bot verify-backup "/backups/vpn-balance-bot-$STAMP.db"
```

## Backup with systemd

```bash
sudo install -d -o vpn-balance-bot -g vpn-balance-bot -m 0700 \
  /var/backups/vpn-balance-bot
STAMP="$(date -u +%Y%m%dT%H%M%SZ)"
sudo -u vpn-balance-bot /usr/bin/env \
  APP_ENV=production \
  DATABASE_PATH=/var/lib/vpn-balance-bot/vpn-balance-bot.db \
  DB_TIMEOUT=30s \
  LOG_LEVEL=info \
  /opt/vpn-balance-bot/vpn-balance-bot backup \
  "/var/backups/vpn-balance-bot/vpn-balance-bot-$STAMP.db"
sha256sum "/var/backups/vpn-balance-bot/vpn-balance-bot-$STAMP.db" \
  | sudo tee "/var/backups/vpn-balance-bot/vpn-balance-bot-$STAMP.db.sha256"

sudo -u vpn-balance-bot /usr/bin/env \
  APP_ENV=production \
  DB_TIMEOUT=30s \
  LOG_LEVEL=info \
  /opt/vpn-balance-bot/vpn-balance-bot verify-backup \
  "/var/backups/vpn-balance-bot/vpn-balance-bot-$STAMP.db"
```

## Backup retention

Use a policy appropriate for the number of customers and recovery requirements.
A practical minimum is:

- seven daily snapshots;
- five weekly snapshots;
- twelve monthly snapshots;
- at least one encrypted off-host copy.

Deletion happens only after the replacement snapshot and its off-host copy are
verified.

## Restore rehearsal

Restore must be rehearsed with an isolated test bot and database before production.
Record the snapshot name, application version, migration state, duration, and
acceptance result.

### Docker Compose restore

Stop the service:

```bash
docker compose down
```

Verify the external checksum:

```bash
sha256sum -c backups/vpn-balance-bot-20260903T120000Z.db.sha256
```

Replace the stopped database volume using a temporary Alpine container. The
existing database is renamed first for local forensic recovery:

```bash
docker run --rm --user 0:0 \
  -v vpn-balance-bot_vpn-balance-data:/data \
  -v "$PWD/backups:/restore:ro" \
  alpine:3.22 \
  sh -eu -c '
    stamp="$(date -u +%Y%m%dT%H%M%SZ)"
    if [ -e /data/vpn-balance-bot.db ]; then
      mv /data/vpn-balance-bot.db "/data/vpn-balance-bot.db.before-$stamp"
    fi
    rm -f /data/vpn-balance-bot.db-wal /data/vpn-balance-bot.db-shm
    cp /restore/vpn-balance-bot-20260903T120000Z.db /data/vpn-balance-bot.db
    chown 65532:65532 /data/vpn-balance-bot.db
    chmod 600 /data/vpn-balance-bot.db
  '
docker compose up -d
docker compose logs --tail 100 bot
```

Use the exact snapshot filename produced by the backup step.

### systemd restore

```bash
sudo systemctl stop vpn-balance-bot.service
sudo install -d -o root -g root -m 0700 \
  "/var/lib/vpn-balance-bot/recovery-20260903"
sudo sh -eu -c '
  for name in vpn-balance-bot.db vpn-balance-bot.db-wal vpn-balance-bot.db-shm; do
    source="/var/lib/vpn-balance-bot/$name"
    if [ -e "$source" ]; then
      mv -- "$source" "/var/lib/vpn-balance-bot/recovery-20260903/$name"
    fi
  done
'
sudo install -o vpn-balance-bot -g vpn-balance-bot -m 0600 \
  /var/backups/vpn-balance-bot/vpn-balance-bot-20260903T120000Z.db \
  /var/lib/vpn-balance-bot/vpn-balance-bot.db
sudo systemctl start vpn-balance-bot.service
sudo journalctl -u vpn-balance-bot.service -n 100 --no-pager
```

The move targets only the main database and its two known WAL sidecar paths, and
only after the service is stopped.

## Post-restore acceptance

1. Startup reports no migration or checksum error.
2. Administrator dashboard totals match the expected snapshot state.
3. A known test account shows the expected balance and recent history.
4. No previously confirmed payment is duplicated.
5. Pending or ambiguous reminders match the recorded pre-backup state.
6. A new backup can be created from the restored database.

Keep the displaced database until acceptance succeeds.
