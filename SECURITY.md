# Security Policy

## Supported versions

Security fixes target the latest published release and the current `main`
branch. Older releases may require upgrading before a fix can be applied.

## Reporting a vulnerability

Do not disclose vulnerabilities in a public issue, discussion, or pull request.

Use the repository **Security** tab and select **Report a vulnerability**:

https://github.com/Nergous/vpn-balance-bot/security/advisories/new

If private vulnerability reporting is unavailable, open a minimal public issue
requesting a private contact channel. Do not include exploit details, tokens,
personal data, logs, database contents, or customer information.

Include, when possible:

- affected release or commit;
- impact and realistic attack conditions;
- minimal reproduction using synthetic data;
- relevant configuration with secrets removed;
- suggested mitigation;
- whether public disclosure has already occurred.

Never test against someone else's bot, Telegram account, host, or database
without explicit authorization.

## Scope

High-value reports include authentication or authorization bypasses, token or
data disclosure, ledger corruption, unsafe migrations or restores, SQL issues,
duplicate financial operations, reminder delivery flaws, and release supply
chain weaknesses.

## Handling and disclosure

Maintainers will validate the report, coordinate a fix and advisory when
appropriate, and keep communication private until disclosure is agreed. No
fixed response or remediation SLA is promised. Credit is provided when desired
and safe to publish.
