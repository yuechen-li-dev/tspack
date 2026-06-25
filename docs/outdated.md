# tspack outdated

`tspack outdated` is a read-only freshness report for declared manifest dependencies.

## Commands

- `tspack outdated`
- `tspack outdated --json`
- `tspack outdated --per-package`

## Behavior

- Reads manifest IR dependency intents.
- Reads `ts-lock.toml` when present for current locked versions.
- Fetches npm package metadata only (no tarball fetch).
- Reports `wanted` (highest satisfying manifest range) and `latest` (registry latest tag or max version).
- Groups identical declarations by default using dependency source/name, kind, requested range, current versions, wanted, latest, status, and warning/error result.
- Grouped rows include `packageCount` and `packages` so monorepos can see how many workspace packages declare the same dependency.
- `--per-package` restores declaration-level output with one row per declaring package.
- `--json` uses grouped `entries` by default and also includes legacy declaration-level `dependencies`; `--per-package --json` makes `entries` declaration-level.
- Non-registry sources are reported as `not_applicable`; workspace dependencies are updated by editing workspace/package manifests rather than by registry outdated checks.

## Exit semantics

- Exit `0` when report completes without metadata/resolution errors.
- Exit non-zero when one or more dependency metadata/resolution errors occur.
- Missing lockfile is warning-only (`TSPACK_OUTDATED_LOCKFILE_MISSING`).

## Mutation contract

`tspack outdated` does not mutate lockfile, store, manifest, or `node_modules`.
