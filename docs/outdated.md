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

## Declared update policy report

A workspace can declare root-level update intent with `<UpdatePolicy />`. In M50a this policy is reporting-only: `tspack outdated` and `tspack outdated --json` annotate candidates, but they do not mutate manifests, lockfiles, stores, or `node_modules`.

Supported strategies are:

- `manual`: update requires an explicit targeted command or manifest/range decision; reported as `blocked-manual`.
- `pinned`: dependency is intended to remain at the current locked version until manifest intent changes; reported as `pinned`.
- `rolling`: dependency may roll within its declared `level`; reported as `allowed` when the latest candidate is inside policy and `outside-policy-level` otherwise.

Rolling levels are `patch`, `minor`, `major`, and `latest`. Rolling rows must declare a level. Dependencies with no matching row are `unclassified`, and non-registry dependencies are `not-applicable`.
