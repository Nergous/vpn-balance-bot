# Changelog

All notable changes to this project will be documented in this file.

The format follows [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and releases use [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

### Added

- Russian and English Telegram interfaces for users and administrators.
- Integer-minor-unit ledger, monthly billing, reminders, and invite tokens.
- Embedded SQLite migrations with checksum verification and online backups.
- Read-only database diagnostics and backup verification commands.
- Docker Compose and hardened systemd deployment options.
- CI, CodeQL, dependency updates, vulnerability scanning, and coverage gates.
- Reproducible release archives, checksums, SBOMs, provenance, and GHCR images.

### Security

- Numeric Telegram authorization and private-chat enforcement.
- Durable Telegram update deduplication and fenced reminder delivery attempts.
- Redacted structured logs that avoid tokens, message text, and filesystem paths.
