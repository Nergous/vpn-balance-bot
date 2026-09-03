# Contributing

Thanks for improving VPN Balance Bot. Keep changes focused, testable, and safe
for existing SQLite installations.

## Development setup

Requirements:

- the Go version declared in `go.mod`;
- Git;
- Docker with Buildx for container changes;
- GNU Make or the equivalent commands from `Makefile`.

```bash
git clone https://github.com/Nergous/vpn-balance-bot.git
cd vpn-balance-bot
go mod download
make ci
make vuln
```

`make ci` checks formatting, module consistency, workflows, vet, race tests,
lint, and the coverage floor.

## Safety rules

- Tests must use temporary SQLite databases.
- Never run tests against a production database or production `.env` file.
- Never use a real Telegram bot token in tests or examples.
- Use fake or local Telegram transports for automated tests.
- Do not commit tokens, database files, backups, logs, or customer data.
- Do not change an already released migration. Add a new migration instead.
- Preserve the integer-minor-unit ledger and immutable financial history.

## Making a change

1. Create a focused branch from `main`.
2. Add or update tests for changed behavior.
3. Update documentation when commands, configuration, deployment, or public
   behavior changes.
4. Add notable user-facing changes under `Unreleased` in `CHANGELOG.md`.
5. Run the relevant checks before opening a pull request.

```bash
make ci
make vuln
docker build --target test .
```

The Docker command is required when the Dockerfile, Compose configuration, or
container-facing behavior changes.

## Database changes

SQLite migrations live in `migrations/` and are embedded into the binary.
Migration versions and checksums are deployment history.

- Add a new ordered migration file; never rewrite an applied migration.
- Prefer additive, backward-compatible changes.
- Include migration, checksum, backup, and restore tests where applicable.
- Explain rollback and compatibility effects in the pull request.

## Commits and pull requests

Use concise Conventional Commits, for example:

```text
fix(storage): preserve ledger consistency
feat(cli): add database diagnostics
docs: clarify backup recovery
```

Pull requests should describe the problem, chosen approach, verification, and
operational impact. Keep unrelated cleanup in a separate pull request.

Security vulnerabilities must follow [SECURITY.md](SECURITY.md), not a public
issue or pull request.
