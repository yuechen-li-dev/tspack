# tspack outdated

`tspack outdated` is a read-only freshness report for declared manifest dependencies.

## Commands

- `tspack outdated`
- `tspack outdated --json`

## Behavior

- Reads manifest IR dependency intents.
- Reads `ts-lock.toml` when present for current locked versions.
- Fetches npm package metadata only (no tarball fetch).
- Reports `wanted` (highest satisfying manifest range) and `latest` (registry latest tag or max version).
- Non-npm sources are reported as `not_applicable`.

## Exit semantics

- Exit `0` when report completes without metadata/resolution errors.
- Exit non-zero when one or more dependency metadata/resolution errors occur.
- Missing lockfile is warning-only (`TSPACK_OUTDATED_LOCKFILE_MISSING`).

## Mutation contract

`tspack outdated` does not mutate lockfile, store, manifest, or `node_modules`.
