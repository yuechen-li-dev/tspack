# TSPack Self-Hosting

## What self-hosting means in 0.1.0

TSPack self-hosting in 0.1.0 means this repository has a real root `manifest.tsx` and TSPack can manage, check, list, and smoke its own repository contract after bootstrap.

It does **not** mean TSPack can build itself from absolute zero, that npm/Bun compatibility files have been removed, or that release CI depends entirely on self-hosting.

## Bootstrap boundary

The accepted bootstrap path is:

1. Build or obtain a `tspack` binary through the existing Go/source path.
2. Build the manifest frontend bridge when running from source: `npm --prefix manifest-frontend run build`.
3. Materialize the self-host toolchain from the committed root lockfile when needed: `go run ./cmd/tspack sync --root .`.
4. Use that binary, or `go run ./cmd/tspack`, to inspect and validate the root repository contract.

The root `check-self`, `check-format-self`, `doctor-security-self`, and `policy-plan-self` RunTargets intentionally use `go run ./cmd/tspack` so source checkouts can dogfood before a public release binary exists.

## Modeled surfaces

| Surface | Manifest package | Modeling approach |
|---|---|---|
| Go CLI/backend (`cmd/tspack`, `internal/*`, `tools/*`) | `@tspack/cli` | Go modules are not modeled as npm dependencies. Repository lifecycle is represented with system RunTargets. |
| Manifest frontend (`manifest-frontend`) | `@tspack/manifest-frontend` | Existing TypeScript tool dependencies are declared as tools while `package.json` and lockfiles remain for bootstrap compatibility. |
| VS Code extension (`extensions/tspack-vscode`) | `@tspack/vscode-extension` | Extension TypeScript tooling is declared conservatively, with compile/test represented as RunTargets. |
| Dogfood examples | `@tspack/examples-runtime-switch-notes`, `@tspack/examples-update-policy-notes` | Examples are represented lightly in the root model; their deeper fixture behavior remains isolated in their own directories. |
| Release/install scripts | `@tspack/cli` RunTargets | Shell scripts remain unchanged and are invoked through explicit `system` runtime targets. |

## Security policy

The root manifest declares a `Security` policy that acknowledges the maintainer-publish lifecycle category. TSPack still blocks dependency lifecycle execution by default; the acknowledgment documents why publish-time metadata should not be confused with consumer install/update execution.

No exact lifecycle capability acknowledgment is included in the root self-host manifest yet, because M52a does not add new reviewed behavior fixtures for this repository root.

## Update policy

The root manifest declares conservative update intent for the TypeScript toolchain:

- `typescript`, `vitest`, and `@types/node` may roll within the current major/minor policy after validation.
- `playwright` remains manual because inspect/browser testing is environment-sensitive and deferred until after 0.1.0.
- `@types/vscode` remains manual because VS Code API compatibility is extension-specific.

`update --policy --dry-run` is read-only and evaluates the declared policy and security gates without rewriting lockfiles.

## Command matrix

Use `go run ./cmd/tspack` from a source checkout, or replace it with a bootstrapped `tspack` binary after one exists.

| Command | Status | Notes |
|---|---|---|
| `go run ./cmd/tspack run --list --root .` | Dogfood smoke | Lists root self-host RunTargets. |
| `go run ./cmd/tspack check --root .` | Dogfood smoke | Validates the self-host manifest contract. |
| `go run ./cmd/tspack check --format --root .` | Dogfood smoke | Read-only format check scoped away from generated artifacts. |
| `go run ./cmd/tspack doctor security --root .` | Dogfood smoke | Reports lifecycle/security posture. |
| `go run ./cmd/tspack doctor security --root . --json` | Dogfood smoke | JSON security report. |
| `go run ./cmd/tspack outdated --root .` | Manual/network | Reports outdated dependency information for modeled npm tooling. |
| `go run ./cmd/tspack outdated --root . --json` | Manual/network | JSON outdated report. |
| `go run ./cmd/tspack update --policy --dry-run --root .` | Dogfood smoke | Read-only policy plan. |
| `go run ./cmd/tspack update --policy --dry-run --json --root .` | Dogfood smoke | JSON read-only policy plan. |
| `go run ./cmd/tspack run frontend-build --root . --once` | Manual target | Runs `npm run build` in `manifest-frontend`. |
| `go run ./cmd/tspack run frontend-typecheck --root . --once` | Manual target | Runs manifest API typecheck. |
| `go run ./cmd/tspack run frontend-test --root . --once` | Manual target | Runs manifest frontend tests. |
| `go run ./cmd/tspack run go-test --root . --once --ready-timeout 300` | Manual target | Runs `go test ./...`. |
| `go run ./cmd/tspack run release-build --root . --once --ready-timeout 300` | Release gate/manual | Runs the release build script and may be expensive. |
| `npm --prefix manifest-frontend run build` | Direct validation | Existing bootstrap path remains intact. |
| `go test ./...` | Direct validation | Existing Go test path remains intact. |
| `./scripts/build-release.sh` | Direct validation | Existing release path remains intact. |

For routine local dogfood, run:

```bash
./scripts/self-host-smoke.sh
```

For the optional release-gate smoke, run:

```bash
./scripts/self-host-smoke.sh --release
```

## No-mutation assertions

The self-host smoke builds the manifest frontend, runs `go run ./cmd/tspack sync --root .` for ignored tool materialization, and records tracked `git status --short --untracked-files=no` before and after the read-only matrix. It fails if `check`, `check --format`, `doctor security`, or `update --policy --dry-run` changes tracked repository state.

Generated build outputs remain ignored by `.gitignore`, including `manifest-frontend/dist/`, extension `dist/`, root `dist/`, `.tspack/`, and `tspack-artifacts/`.

## Current limitations

- `package.json`, `package-lock.json`, and Bun/npm compatibility files are intentionally not removed.
- The setup action is not used to bootstrap this repository before the first real release exists.
- Inspect/browser deep testing is deferred until after 0.1.0 and should be run locally with real browser/VS Code context.
- Policy-driven mutation is not implemented.
- Go modules are not modeled as npm dependencies.
- The root manifest does not claim a zero-bootstrap build.

## Verdict

Outcome A for M52a: the repository has a root self-host manifest, core surfaces are modeled, RunTargets expose the command matrix, security/update policy intent is declared, and `scripts/self-host-smoke.sh` verifies the read-only dogfood path after bootstrap.
