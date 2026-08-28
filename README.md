# TSPack

TSPack is an early TypeScript project lifecycle manager built around explicit manifests, deterministic lockfiles, blocked-by-default lifecycle scripts, and policy-aware updates.

## Status

The current release is `v0.1.8`. TSPack is useful for early dogfooding and release-candidate validation, but it is not production-stable and the CLI, manifest API, and generated artifacts may still change.

The v0.1.8 focus is operational trust: structured manifest failures, explicit minimum-version contracts, actionable Windows file-lock diagnostics, dependency-change attribution, and native OSV vulnerability auditing.

The repository is self-hosted after bootstrap: a trusted source checkout or binary is required first, then TSPack can manage, check, audit, and describe its own project contract. See [self-hosting](docs/self-hosting.md) and the [release gate](docs/release-gate.md).

## Why

- npm lifecycle scripts can execute arbitrary package code by default.
- TSPack detects dependency lifecycle capabilities and blocks lifecycle execution by default.
- Package and update intent should be declared and reviewed instead of outsourced to unbounded bot churn.
- `manifest.tsx` is the project contract; `ts-lock.toml` is resolved truth.

## Install

Download release artifacts from GitHub Releases. Release archives are expected to include SHA256 coverage through `checksums.txt`.

From a trusted checkout, Unix users can run:

```bash
TSPACK_VERSION=v0.1.8 sh scripts/install.sh
```

The installer downloads GitHub Release artifacts, verifies the checksum entry, and installs `tspack` to `$HOME/.local/bin` unless `TSPACK_INSTALL_DIR` is set. Review installer scripts before running them from raw URLs. `get.tspack.dev` is not live.

TSPack resolves npm and JSR registry packages directly and does not manage Node.js runtime versions. JSR use does not require Deno or a package-manager switch. Use a Node already on `PATH`; if you want a runtime manager, `mise` is the recommended option: https://mise.jdx.dev/

## Quickstart

```bash
tspack init --kind library --name my-package
# init also writes tsconfig.tspack.json and local manifest/xTest editor types.
tspack add lodash
tspack add @std/path --source jsr
tspack remove lodash
tspack update
tspack check
tspack check --format
tspack doctor security
tspack outdated
tspack update --policy --dry-run
```

Useful release sanity checks:

```bash
tspack --version
tspack
tspack help workflow
tspack help concepts
tspack help commands
```

## Core features

- `manifest.tsx` / `package.manifest.tsx` project contracts.
- Deterministic `ts-lock.toml` lockfiles.
- Mixed npm + JSR resolution through source-qualified registry backends, with no Deno requirement.
- First-class npm-default and explicit `--source jsr` add/remove through semantic authoring IR and source-preserving manifest projection; registries are never auto-searched.
- Declarative registry source policy with source allowlists, ordered npm-compatible mirrors/fallback, endpoint-scoped environment credentials, trust checks, endpoint provenance, and offline store reuse.
- `update`, `sync`, and content-addressed store population.
- `check` and read-only `check --format` validation.
- Blocked-by-default dependency lifecycle security policy.
- `doctor security` lifecycle capability audit surface.
- Native `tspack audit` checks locked npm versions against OSV.dev without invoking npm or mutating dependency state.
- Declared RunTargets with runtime inheritance.
- Native xTest harness.
- `outdated` plus declared UpdatePolicy dry-run planning.
- `pack`, `why`, and `how` release/audit helpers.
- Per-target compiler orchestration: ordinary tsc projects may opt a bounded
  source set into ScriptC native-executable compilation without becoming
  project-wide ScriptC. See [M71a ScriptC hot paths](docs/dev/m71a-scriptc-hotpath.md).
- Perry uses the same bounded target model through a project-managed tool and
  Perry-owned config. See [the M71b performance qualification](docs/dev/m71b-perry-performance.md).

## Self-hosting

TSPack is self-hosted after bootstrap. The root `manifest.tsx` and `ts-lock.toml` describe this repository, and `./scripts/self-host-smoke.sh` exercises the intended dogfood path. See [docs/self-hosting.md](docs/self-hosting.md).

## Known limitations

- Early patch release; not production-stable.
- Browser-required inspect tests may skip when Playwright Chromium is unavailable.
- Policy-driven update mutation is not implemented; `update --policy --dry-run` is read-only planning.
- The `setup-tspack` action is release-backed and should still be validated against real published artifacts as part of release prep.
- Homebrew, mise, npm bootstrapper, and `get.tspack.dev` distribution channels are future work.
- ScriptC M71a uses a coarse native sidecar boundary. The measured Windows x64
  integer kernel did not beat Node/V8, so projects must benchmark their real
  workload; TSPack does not claim that native output is automatically faster.

## Docs

Start with [docs/README.md](docs/README.md), especially the [v0.1.8 release notes](docs/releases/v0.1.8.md), [native audit guide](docs/audit.md), [roadmap](docs/roadmap.md), [distribution](docs/distribution.md), [security](docs/security.md), and [release gate](docs/release-gate.md).

### Quickstart template

```sh
tspack init --template static --name hello-static
tspack update
tspack sync
tspack check

# or start a practical frontend app:
tspack init --template react --name my-app
tspack update
tspack sync
tspack run dev
```

The default static template creates a minimal TypeScript browser app. The explicit React template creates a React + Vite + TypeScript app. `tspack init` prints next-step hints, and `tspack help workflow`, `tspack help concepts`, and `tspack help init` explain the lifecycle without requiring source-code reading. Remote template workflows are planned later.

Cold `tspack update` and `tspack sync` runs may print plain `[i/n]` progress lines while store artifacts are fetched or packages are materialized.


Built-in templates include `static`, `react` for React + Vite apps, and `react-library` for React + Vite component libraries.
