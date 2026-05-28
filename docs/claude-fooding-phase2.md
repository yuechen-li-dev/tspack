# Claude-Fooding Phase 2 Remediation

## Original findings

Claude-fooding Phase 2 exercised the package-manager critical path and found production blockers plus UX gaps in the lock/update/sync loop:

- update→sync gap
- no production npm HTTP client/store population
- tarball extraction prefix bug
- workspace/path root resolution bug
- diagnostic details not printed
- why duplicate lock edges
- why transitive query UX
- duplicate locked versions not warned
- update --dry-run missing
- outdated missing
- targeted update missing
- update progress missing

## Remediation summary

| Finding | Milestone | Result | Status |
|---|---|---|---|
| update→sync gap | M32a | `tspack update` now prepares the lock/store state consumed by `tspack sync`. | Complete |
| no production npm HTTP client/store population | M32a | Production npm metadata/tarball fetching and content-addressed store population are part of update. | Complete |
| tarball extraction prefix bug | M32a | Package tarballs strip the first archive component before store extraction/materialization. | Complete |
| workspace/path root resolution bug | M32a | Workspace and path dependencies resolve relative to the active root directory. | Complete |
| diagnostic details not printed | M32a | Human diagnostics print detail lines for resolver/store troubleshooting. | Complete |
| why duplicate lock edges | M32b | `tspack why` deduplicates lock edge output while preserving deterministic order. | Complete |
| why transitive query UX | M32b | Bare-name misses now guide users toward matching lock package IDs. | Complete |
| duplicate locked versions not warned | M32c | `tspack check` emits `TSPACK_LOCK_VERSION_CONFLICT` warnings with `how` guidance. | Complete |
| update --dry-run missing | M32d | `tspack update --dry-run` reports human/JSON lock diffs without lock/store/node_modules mutation. | Complete |
| outdated missing | M32e | `tspack outdated` reports metadata-only freshness in text or JSON without tarball/store/materialization mutation. | Complete |
| targeted update missing | M32f | `tspack update <query>` supports key/name/`npm:<name>` declared-dependency selection and preserves non-selected roots when valid. | Complete |
| update progress missing | M32g | Update and dry-run progress is stderr-only, supports targeted context, preserves JSON stdout cleanliness, and honors `--quiet`. | Complete |

## Current golden path

A Phase 2 release smoke should validate the package-manager loop with commands like:

```bash
tspack outdated
tspack update --dry-run
tspack update <query>
tspack update
tspack sync
tspack check --json
tspack why <query>
tspack how <diagnostic>
```

The critical path is now validated as:

```text
update -> store -> sync
```

## Explicit non-goals

- no lifecycle scripts
- no npm/npx compatibility mode
- no semantic hoisting
- no convergence policy yet
- no automatic dedupe yet

## Deferred future work

- package-family convergence policies
- richer update progress/verbose mode if needed
- update policy tuning
- possible `why --json` / `why --reverse` later

## Final status

Phase 2 remediation is complete. The closeout state is **Success**: the original motivating workflow now has a validated update→store→sync loop, read-only planning/freshness commands, targeted updates, clearer diagnostics, and release-gate smoke coverage expectations.
