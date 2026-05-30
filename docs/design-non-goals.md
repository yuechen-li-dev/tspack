# Design scope and non-goals (M24)

## Intentionally supported now

TSPack intentionally supports:
- package lifecycle commands (`check`, `update`, `sync`, `why`, `pack`)
- native test harness (`test`, xTest/Vitest orchestration)
- standalone artifacts (`artifact`)
- benchmark units (`bench`)
- doom/prophecy units (`doom`)
- declared runtime targets (`run`)
- structural inspection via `inspect` (**experimental**)

## Still out of scope

TSPack is still not:
- npm script compatibility layer
- arbitrary task runner
- build/bundler framework
- publish/deployment workflow
- lifecycle script executor by default
- Storybook clone
- screenshot visual testing system
- machine vision product
- automatic app attach / local port scanner
- complete semantic migration from package.json/script/config soup

## Clarifying boundaries

- `run` executes declared `RunTargets` from manifest contract, not `package.json` scripts.
- `migrate` is an onboarding draft generator. It translates mechanical package.json facts and marks uncertainty; it is not a lockfile translator, source scanner, build runner, script executor, or architectural inference engine.
- `inspect` is an experimental inspection loop, not a stable browser automation API.


- `tspack format` and `tspack lint` are Biome-backed lifecycle UX commands. See `docs/format-lint.md`.

- `tspack doctor` adds non-mutating environment diagnostics. See `docs/doctor.md`.


- VS Code extension and standalone LSP are not required for manifest authoring in M31f.
- Manifest helper authoring surface is not runtime helper execution.
