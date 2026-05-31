# Runtime Profile Closeout

## Original motivation

TypeScript projects should not become npm-shaped, Bun-shaped, or Deno-shaped by accident. Bun and Deno intentionally bundle runtime execution, package management, tasks, tests, cache/vendor behavior, and ecosystem conventions into one toolchain. That is useful for those tools, but it is not TSPack's project contract.

TSPack separates runtime identity from lifecycle truth. Dependency resolution, lockfiles, materialization, checks, pack output, lifecycle security, and package-manager semantics remain TSPack-owned. Runtime profiles are the portability axis for backend/runtime identity: change runtime profile, keep project contract.

## Runtime profile thesis

`nodejs`, `bun`, and `deno` are runtime profiles. `npm`, `pnpm`, and `yarn` are package-manager ecosystems, not runtime profiles, and are rejected when used as `<Workspace runtime="...">` values.

TSPack owns:

- dependency resolution
- `ts-lock.toml`
- materialization
- checks
- pack and pack verification
- lifecycle security policy
- package-manager semantics

The runtime profile currently affects reporting and appropriate explicit runtime execution paths. Workspace runtime does not yet imply RunTarget inheritance. Explicit `RunTarget.runtime` wins, so a workspace declared as `runtime="deno"` can still launch an explicitly declared `runtime: "bun"` target with Bun.

## Remediation / implementation summary

| Need / finding | Milestone | Fix | Status |
|---|---|---|---|
| Define the runtime profile axis without turning npm/pnpm/yarn into runtimes. | M42a | Added `<Workspace runtime="nodejs" | "bun" | "deno">`, defaulted omitted runtime to `nodejs`, rejected package-manager-like runtime values, added type surface and Go IR validation/defaulting, and taught `tspack doctor runtime` to report selected profile, executable, availability, status, lifecycle owner, and `packageManagerDelegated: false`. | Complete |
| Preserve existing Node.js behavior before adding Bun/Deno proofs. | M42b | Locked `nodejs` as the compatibility baseline, proved omitted runtime and explicit `runtime="nodejs"` normalize equivalently, preserved check/pack/why/run/native xTest/doctor behavior, prevented runtime profile leakage into package metadata, and proved workspace runtime does not override explicit RunTarget runtime. | Complete |
| Prove Bun as a constrained runtime execution path without Bun package-manager semantics. | M42c | Added explicit `RunTarget.runtime: "bun"` launch as `bun <declared argv>`, deterministic missing-Bun failure, and guardrails against Bun install/add/pm, Bun lockfile, resolver/materializer changes, xTest/JS bridge switching, and workspace-runtime inheritance. | Complete |
| Prove Deno as a constrained runtime execution path without Deno task/package/cache semantics. | M42d | Added explicit `RunTarget.runtime: "deno"` launch as `deno <declared argv>`, deterministic missing-Deno failure, and guardrails against Deno task/install/add/cache/vendor, deno.json/deno.lock, import maps, JSR/npm: resolver behavior, xTest/JS bridge switching, and workspace-runtime inheritance. | Complete |
| Demonstrate one-line runtime switching while keeping the project contract stable. | M42e | Added nodejs/bun/deno runtime switch demo fixtures, proved manifests differ only by `<Workspace runtime>`, normalized IR differs only in `Workspace.Runtime`, check/pack/why/package metadata remain stable, `doctor runtime` selected profile changes, explicit node/bun/deno/system RunTargets stay explicit and stable, and package-manager delegation guardrails remain in place. | Complete |

## Current runtime model

- Workspace runtime is optional and accepts only `"nodejs"`, `"bun"`, or `"deno"`.
- Omitted workspace runtime defaults to `nodejs`.
- `tspack doctor runtime` reports the selected profile, executable, selected-runtime availability, status, lifecycle ownership, and package-manager delegation status.
- Explicit RunTarget runtime remains the execution contract:
  - `runtime: "node"` keeps existing Node/local-bin behavior.
  - `runtime: "system"` keeps existing direct argv behavior.
  - `runtime: "bun"` invokes `bun <declared argv>`.
  - `runtime: "deno"` invokes `deno <declared argv>`.
- Missing explicit Bun or Deno executables fail before child execution with `TSPACK_RUN_RUNTIME_NOT_FOUND`.
- The one-line runtime switch fixture family is `fixtures/valid/runtime-switch-nodejs`, `fixtures/valid/runtime-switch-bun`, and `fixtures/valid/runtime-switch-deno`.
- Runtime profile does not leak into generated `package.json` or pack metadata.
- Runtime profile does not delegate package-manager work to npm, pnpm, yarn, Bun, or Deno.

## Current golden workflow

```sh
tspack doctor runtime
tspack check
tspack pack --verify
tspack why <dep>
tspack run bun-hello --once
tspack run deno-hello --once
```

The one-line switch is the workspace runtime value:

```tsx
<Workspace runtime="nodejs">
```

```tsx
<Workspace runtime="bun">
```

```tsx
<Workspace runtime="deno">
```

What changes:

- `doctor runtime` selected runtime profile and selected executable availability.
- Explicit runtime availability/execution where a RunTarget declares that runtime.

What does not change:

- dependency graph
- lockfile semantics
- materialization
- `tspack check`
- `tspack pack`
- `tspack why`
- package metadata
- lifecycle security policy
- format/lint behavior
- native xTest bridge
- inspect/JavaScript bridge

## Explicit non-goals / deferred work

- no npm/pnpm/yarn package-manager switching
- no `bun install`, `bun add`, or `bun pm`
- no `deno task`, `deno install`, `deno add`, `deno cache`, or `deno vendor`
- no deno.json, deno.lock, import maps, or JSR support
- no workspace runtime inheritance into RunTargets yet
- no native xTest runtime switching
- no JavaScript bridge runtime switching
- no resolver/materializer changes
- no runtime profile package metadata
- no package-manager mutation

## Future ladder

Possible future work, if explicitly designed:

- workspace runtime inheritance/defaulting for RunTargets
- runtime-profile-aware native xTest backend
- runtime-profile-aware JavaScript bridge backend
- Deno import-map/JSR design, if justified
- Bun compatibility tests beyond explicit runtime execution
- runtime switch demo in docs/site
