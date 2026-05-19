# TSPack Contract (current)

TSPack is a TypeScript lifecycle tool with package lifecycle guarantees at the core.

- Intent: `manifest.tsx`
- Resolved truth: `ts-lock.toml`
- Compatibility materialization: `node_modules`
- Security model: fetch-not-execute by default

Layered scope:
1. Core package lifecycle (`check`, `update`, `sync`, `why`, `pack`)
2. Native development harness (`test`, `artifact`, `bench`, `doom`)
3. Runtime/inspection loop (`run`, `inspect` experimental)

Guardrails:
- `run` starts declared run targets; it is not npm script execution.
- `inspect` is experimental.
- `check/sync/pack/why/run/inspect` do not mutate lockfile by contract.
- `update` is the lockfile mutation command.
