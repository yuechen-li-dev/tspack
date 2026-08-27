# Lockfile (`ts-lock.toml`) in M6

`manifest.tsx` expresses intent while `ts-lock.toml` records resolved truth.

## Principles
- Deterministic and stable ordering.
- Human-reviewable and hash-pinned.
- Parse/write round-trips are stable.
- `sync` / `check` / `pack` must not mutate lockfiles (enforced in later milestones).

## M6 scope
M6 only implements lockfile data modeling, TOML parse/write, deterministic sorting, semantic validation, graph-vs-lock target consistency checks, and semantic diffing.

Non-goals: resolver, npm/git fetching, update/sync CLI flows, store, `node_modules`, pack/why/build/test/dev/publish.

## TOML shape
- `[lock]` header with `format=1`, `tool="tspack"`.
- `[[package]]` entries for source-pinned packages.
- `[[edge]]` entries connecting source graph nodes to resolved package ids.
- optional `[[requirement]]` entries preserving shared-slot intent,
  provenance, precedence outcome, and compatibility classification.
- `[[target]]` entries for target outputs.

M70x keeps `format=1` and adds requirement/reference fields additively. An edge
`reference` is the local materialization spelling for an npm alias; its `to`
package ID remains the semantic source-qualified target. Requirement rows make
offline `why` and `check` reproducible without treating losing requirements as
installed packages.

## Package capabilities

Package entries may contain nested capability records. M37a records npm lifecycle scripts as explicit capabilities:

```toml
[[package]]
id = "npm:esbuild@0.24.0"
name = "esbuild"
version = "0.24.0"
source = "npm"

  [[package.capability]]
  kind = "lifecycleScript"
  script = "postinstall"
  command = "node install.js"
```

Capabilities are sorted deterministically by package ID and then by capability kind, script, and command. Old lockfiles without capability metadata still parse as packages with no capabilities. `update` writes refreshed capability metadata; `check` and `sync` do not rewrite a lockfile solely to add missing capabilities.
