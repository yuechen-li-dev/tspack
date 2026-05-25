# `tspack init` (M30)

`tspack init` scaffolds a readable starter `manifest.tsx` and a matching source entry file.

## Kinds

- `--kind library` creates `src/index.ts` and a library-oriented manifest target.
- `--kind app` creates `src/main.ts` and an app-oriented manifest target with relaxed type policy defaults.

## Generated files

- `manifest.tsx`
- `src/index.ts` for libraries
- `src/main.ts` for apps

## Flags

- `--kind <library|app>` (required)
- `--name <package-name>` (required)
- `--version <version>` (default: `0.1.0`)
- `--license <license>` (default: `MIT`)
- `--force` (overwrite generated files)
- `--dry-run` (print plan only)

## Safety

By default, `tspack init` refuses to overwrite existing generated files and emits `TSPACK_INIT_FILE_EXISTS`.

`--force` overwrites only generated paths; it does not delete unrelated files.

## Non-goals

`init` does **not**:

- install dependencies
- run `tspack update` or `tspack sync`
- generate `ts-lock.toml`
- create `node_modules`
- build output artifacts

## Next commands

- `tspack check`
- `tspack update`
- `tspack sync`
- `tspack run` (after manually declaring `RunTargets`)
