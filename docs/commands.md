# Commands (M13)

- `tspack check`: validates manifest/frontend, IR, graph, boundaries, type surfaces, and lock consistency when lockfile exists. It does **not** write lockfile, store artifacts, or `node_modules`.
- `tspack update`: resolves sources and writes `ts-lock.toml` deterministically.
- `tspack sync`: requires an existing lockfile, validates lock consistency against graph, then materializes strict `node_modules` from store artifacts. It never mutates `ts-lock.toml`.
- `tspack pack [--root .] [--out <dir>] [--package <name>] [--dry-run]`: creates deterministic local package archives.
- `tspack why <query> [--root .] [--manifest <path>] [--lockfile <path>] [--package <name>]`: explains why a dependency/target/lock package is present.
