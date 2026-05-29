# Format and lint commands

TSPack provides Biome-backed code formatting and linting orchestration.

- `tspack format [paths...] [--root .] [--check]`
- `tspack lint [paths...] [--root .] [--fix]`

## Backend resolution

TSPack resolves the Biome backend in this order:

1. `<root>/node_modules/.bin/biome`
2. `<root>/node_modules/@biomejs/biome/bin/biome`
3. `biome` from `PATH`

The direct package fallback supports strict TSPack materialization layouts where the package binary exists even if a package-manager-style shim is unavailable. TSPack does not use `npm run`, `npx`, `bunx`, `pnpm dlx`, or `yarn dlx`.

If Biome is missing, TSPack emits `TSPACK_BIOME_BACKEND_NOT_FOUND`.

## Config behavior

- If `biome.json` or `biome.jsonc` exists in project root, Biome uses that config.
- If neither config file exists, TSPack generates a temporary default config and passes it with `--config-path` for the command invocation.
- The temporary config file is not written into project root and is cleaned up after execution.

## Mutation behavior

- `tspack format` may modify files by invoking Biome format with `--write`.
- `tspack format --check` is read-only and uses Biome's non-write format mode. TSPack does not pass a Biome `--check` flag for `format`.
- `tspack lint` is read-only.
- `tspack lint --fix` may modify files (`--write`).

These commands do not install packages or run package-manager scripts.

## Relationship to `tspack check`

`check` remains architecture/manifest/lock/type/boundary validation and does not include linting or formatting.

## Sync compatibility expectation

When Biome is declared as a direct tool dependency and materialized by `tspack sync`, TSPack generates `node_modules/.bin/biome` as part of strict compatibility materialization and preserves the executable package binary at `node_modules/@biomejs/biome/bin/biome` on POSIX. `tspack format`/`tspack lint` can resolve either local backend without npm script execution.
