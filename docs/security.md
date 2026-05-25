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
