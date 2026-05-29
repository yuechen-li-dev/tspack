# Security and capability policy (M14)

## Core principle: fetch is not execute

TSPack fetches package metadata/content and records deterministic lockfile truth.
It does not execute package lifecycle scripts.

Allowed behavior:
- fetch metadata
- fetch tarballs
- read `package.json`
- inspect lifecycle scripts
- record package capabilities in `ts-lock.toml`
- materialize files into `node_modules`

Forbidden behavior:
- running `preinstall/install/postinstall`
- running `prepare/prepack/postpack/prepublish`
- invoking shell/node/powershell to execute dependency package code
- native addon compilation as install side effects
- binary download side effects from lifecycle scripts

## Capability model

Lockfile package entries include:

- `capability.kind`
- `capability.detail`

Lifecycle script capability records use:

- `kind = "lifecycle-script"`
- `detail = "<script-name>"`

Detected lifecycle script names:

- `preinstall`
- `install`
- `postinstall`
- `prepublish`
- `prepare`
- `prepack`
- `postpack`

Capabilities are sorted/deduplicated deterministically and round-trip through lockfile parse/marshal.

## Visibility

- `tspack update` produces lockfile changes when capabilities change.
- `tspack check` warns with `TSPACK_CAPABILITY_LIFECYCLE_SCRIPT_PRESENT` when lockfile packages include lifecycle capabilities.
- `tspack update` may fetch npm tarballs and populate the store, but it never executes package code or lifecycle scripts.
- `tspack update --dry-run` may fetch registry metadata for version resolution but does not fetch/store tarballs or materialize `node_modules`.
- `tspack sync` materializes files only and never executes scripts.

## Current policy status

- Lifecycle scripts are blocked by design (never executed).
- No script allowlist execution exists in v1.
- No capability approval execution flow exists in v1.

## Non-goals for M14

- vulnerability scanning
- license scanning
- native build support
- binary download support
- script execution

- `tspack outdated` fetches registry metadata only; it does not fetch tarballs or execute scripts.

## Run environment overlays

`tspack run --env KEY=VALUE` passes explicit values to the child process environment after inheriting the parent environment. These values are process inputs, not secret-manager entries: TSPack does not redact child output, store secrets, load dotenv files, or provide approval semantics for environment variables.

TSPack status output prints only environment keys, such as `Env: PORT, NODE_ENV`, and never prints overlay values itself. The child process can still print any environment value to stdout or stderr, and those streams pass through unchanged.

TSPack does not perform shell expansion, variable interpolation, or quote stripping for `--env`; the value is whatever argv contains after the user's shell has run.


## Pack artifact verification

`tspack pack --verify` is a non-executing structural check of the produced npm tarball. It opens the generated archive, parses `package/package.json`, validates metadata and referenced file paths, and checks peer dependency metadata without running package code, lifecycle hooks, package scripts, `npm install`, publish flows, or network registry checks.
