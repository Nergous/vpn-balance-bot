## Summary

<!-- What changed? -->

## Motivation

<!-- What problem does this solve? -->

## Verification

<!-- List commands and results. Tests must use temporary SQLite databases. -->

## Operational impact

<!-- Configuration, migration, backup, deployment, or rollback effects. -->

## Checklist

- [ ] The change is focused and excludes unrelated cleanup.
- [ ] Tests use temporary SQLite databases and fake or local Telegram transports.
- [ ] No token, credential, production database, backup, log, or private data is included.
- [ ] `make ci` passes, or the reason it cannot run is documented above.
- [ ] `make vuln` passes, or the result is documented above.
- [ ] Docker checks pass when container behavior changes.
- [ ] Documentation and `CHANGELOG.md` are updated for user-facing changes.
- [ ] Database migrations are additive and existing released migrations are unchanged.
- [ ] Security-sensitive details are reported privately instead of placed in this pull request.
