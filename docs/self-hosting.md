# TSPack Self-Hosting

TSPack 0.1.0 self-hosting is intentionally modest: after an existing source or binary bootstrap produces a usable `tspack`, this repository uses TSPack to describe, check, and smoke its own repository contract.

## What self-hosting means in 0.1.0

- The repository has a root `manifest.tsx` and committed root `ts-lock.toml` self-host lock artifact.
- The root workspace declares `name="tspack"` and `runtime="nodejs"`.
- RunTargets model the practical repository lifecycle: frontend build/typecheck/test, Go tests, self-checks, policy planning, and release build.
- Root `Security` and `UpdatePolicy` declarations document the repository's lifecycle and dependency-update posture.
- The self-host path is a dogfood and release-gate surface: it proves the repo contract can be parsed, checked, listed, audited, and policy-planned by TSPack itself after bootstrap.

## What self-hosting does not mean yet

- TSPack does not build itself from absolute zero.
- `package.json`, package lockfiles, Bun/npm compatibility files, and npm scripts are not removed.
- Go modules are not modeled as npm dependencies.
- Release CI is not fully replaced by TSPack.
- The `setup-tspack` action live smoke waits until a public release exists.
- Inspect/browser deep testing is deferred until after 0.1.0.
- Policy-driven mutation is not implemented; policy planning is read-only.

## Bootstrap flow

From a fresh source checkout, use the source bootstrap path explicitly:

```bash
cd manifest-frontend && npm run build
cd ..
go run ./cmd/tspack --help
./scripts/self-host-smoke.sh
```

Those commands use `go run ./cmd/tspack` through the smoke script so a checkout can dogfood before a public binary exists.

If a built binary is preferred, build it first and then run read-only inspection with that binary:

```bash
./scripts/build-release.sh
./dist/tspack run --list --root .
```

The root self-host RunTargets still intentionally use `go run ./cmd/tspack` where that keeps source-checkout dogfooding independent of release publication.

## Modeled surfaces

| Surface | Manifest package | Modeling approach |
|---|---|---|
| Go CLI/backend (`cmd/tspack`, `internal/*`, `tools/*`) | `@tspack/cli` | Go modules are not modeled as npm dependencies. Repository lifecycle is represented with explicit `system` RunTargets. |
| Manifest frontend (`manifest-frontend`) | `@tspack/manifest-frontend` | Existing TypeScript tool dependencies are declared as tools while `package.json` and lockfiles remain for bootstrap compatibility. |
| VS Code extension (`extensions/tspack-vscode`) | `@tspack/vscode-extension` | Extension TypeScript tooling is declared conservatively, with compile/test represented as RunTargets. |
| Dogfood examples | `@tspack/examples-runtime-switch-notes`, `@tspack/examples-update-policy-notes` | Examples are represented lightly in the root model; deeper fixture behavior remains isolated in their own directories. |
| Release/install scripts | `@tspack/cli` RunTargets | Shell scripts remain unchanged and are invoked through explicit `system` runtime targets. |

## Self-host command matrix

Use `go run ./cmd/tspack` from a source checkout, or replace it with a bootstrapped `tspack` binary after one exists.

| Command | Purpose | Expected mutation behavior | Routine or release/manual |
|---|---|---|---|
| `tspack run --list --root .` | Lists root self-host RunTargets. | Read-only. | Routine smoke. |
| `tspack check --root .` | Validates the root manifest and lock contract. | Read-only. | Routine smoke. |
| `tspack check --format --root .` | Validates formatting without rewriting files. | Read-only. | Routine smoke. |
| `tspack doctor security --root . --json` | Audits lifecycle/security posture in machine-readable form. | Read-only. | Routine smoke. |
| `tspack outdated --root .` | Reports outdated modeled npm tooling. | Read-only, may require network/registry access. | Manual/release review. |
| `tspack update --policy --dry-run --root . --json` | Evaluates update policy and security gates without writing. | Read-only. | Routine smoke. |
| `tspack run frontend-build --root .` | Runs the manifest frontend build RunTarget. | May create ignored build output. | Manual/release validation. |
| `tspack run frontend-typecheck --root .` | Runs manifest API typechecking. | Read-only except tool caches. | Manual/release validation. |
| `tspack run frontend-test --root .` | Runs manifest frontend tests. | Read-only except test/tool caches. | Manual/release validation. |
| `tspack run go-test --root .` | Runs `go test ./...`. | Read-only except Go test/cache behavior. | Manual/release validation. |
| `./scripts/self-host-smoke.sh --release` | Runs routine smoke plus the release build script. | May create ignored release artifacts. | Optional release gate/manual. |

Routine local dogfood:

```bash
./scripts/self-host-smoke.sh
```

Optional release-gate smoke:

```bash
./scripts/self-host-smoke.sh --release
```

## No-mutation contract

The read-only self-host smoke records `git status --short --untracked-files=no` before and after the matrix and fails if tracked state changes. Ignored generated artifacts may still be created, including frontend build output, `.tspack/`, `dist/`, and `tspack-artifacts/`. If Biome is not already installed, the smoke creates an ignored `.tspack/self-host-bin/biome` wrapper that uses `npm exec` for the format-check backend without committing tool artifacts.

The root `ts-lock.toml` is committed intentionally. It is the deterministic self-host lock artifact, not an accidental generated file.

## Security posture

Dependency lifecycle execution remains blocked by default. The root `Security` policy currently acknowledges only the maintainer-publish lifecycle category so publish-time metadata is not confused with consumer install/update execution.

No exact root behavior fixture evidence was invented during M52a. The supported audit surface is `tspack doctor security --root .` and, for automation, `tspack doctor security --root . --json`.

## Update policy posture

The root `UpdatePolicy` is conservative. Rolling toolchain dependencies are planning/reporting only in 0.1.0, and `tspack update --policy --dry-run --root . --json` is read-only.

Security gates are evaluated as part of policy planning. Policy-driven mutation, targeted policy planning, and broader coherence policies are post-0.1.0 work.

## Known limitations and deferred items

- Phase 11 inspect/browser deep testing.
- Policy-driven mutation and targeted policy planning.
- React/single-version coherence policy.
- Homebrew, mise, npm, or hosted bootstrapper distribution channels.
- `check --lint`, per-file format diagnostics, and pre-commit hook generation.
- Visionary / VS Code fork work.
- `setup-tspack` hosted smoke after the first release exists.
- `get.tspack.dev` hosted installer endpoint.

## Verdict

Outcome A target for M52b: the self-hosting story is honest, reproducible after bootstrap, documented, release-gateable, and covered by a smoke script that fails on unexpected tracked mutation.
