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

- `tspack doom [--root .] [--list] [--filter <text>] [--json] [--out <path>]`: runs quarantined abnormal-termination Prophecy units from `*.prophecy.tsx` in child processes.

See also `docs/doom.md` for Prophecy/Doom behavior and limits.


## inspect (experimental)

`inspect` is currently experimental/unstable.

`tspack inspect <url>` performs structural UI inspection for rendered browser targets. It also supports declared run targets (`tspack inspect dev` / `tspack inspect --run dev`) that are started, waited for readiness, inspected, then shut down. It supports explicit installed host launch via `--host-path` (with `--browser-path` compatibility alias) and explicit CDP endpoint attach via `--cdp`.

See `docs/inspect.md` for backend taxonomy, diagnostics, and current environment limitations.


## run

`tspack run [target]` starts a declared `<RunTargets />` runtime target, waits for HTTP readiness, then streams logs until interrupted. Use `--once` for smoke checks. It is **not** an npm script runner. See `docs/run.md`.
