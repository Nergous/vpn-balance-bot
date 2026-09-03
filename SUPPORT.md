# Support

Start with these documents:

- [README](README.md)
- [Production operations](docs/operations.md)
- [Backup and restore](docs/backup-and-restore.md)
- [Troubleshooting](docs/troubleshooting.md)
- [Database design](docs/database.md)

Useful diagnostics:

```bash
./vpn-balance-bot version
./vpn-balance-bot doctor
./vpn-balance-bot migrate-status
docker compose ps
docker compose logs --tail=200 bot
```

Use the support issue form for configuration and deployment questions. Use the
bug form only for reproducible defects and the feature form for proposals.

Never attach `.env` files, Telegram tokens, production databases, backups,
customer data, or unredacted logs. Replace Telegram IDs, filesystem paths, and
message contents with synthetic values.

Security vulnerabilities must be reported privately according to
[SECURITY.md](SECURITY.md).

Support is community-maintained and has no guaranteed response time.
