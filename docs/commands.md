# Commands (M13)

- `tspack check`: validates manifest/frontend, IR, graph, boundaries, type surfaces, and lock consistency when lockfile exists. It warns on lifecycle-script capabilities (`TSPACK_CAPABILITY_LIFECYCLE_SCRIPT_PRESENT`). It does **not** write lockfile, store artifacts, or `node_modules`.
- `tspack update`: resolves sources and writes `ts-lock.toml` deterministically.
- `tspack sync`: requires an existing lockfile, validates lock consistency against graph, then materializes strict `node_modules` from store artifacts. It never mutates `ts-lock.toml`.
- `tspack pack [--root .] [--out <dir>] [--package <name>] [--dry-run]`: creates deterministic local package archives.
- `tspack why <query> [--root .] [--manifest <path>] [--lockfile <path>] [--package <name>]`: explains why a dependency/target/lock package is present.
- `tspack test [--root .] [-xtest|--xtest] [-vitest|--vitest] [--list] [--filter <text>]`: runs test backends (xTest and/or Vitest).


## Standalone artifacts

- `tspack artifact
- `tspack bench [--root .] [--list] [--filter <text>] [--json]`: runs native benchmark units from `*.benchmark.tsx`.
` runs standalone native xTest `<Artifact>` units declared directly under `<Suite>`.
- `tspack pack` creates package `.tgz` archives; it is unrelated to native test artifacts.


See also `docs/artifacts.md` for standalone artifact mode details.
