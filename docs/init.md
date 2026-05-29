# `tspack init` (M30)

`tspack init` scaffolds a readable starter `manifest.tsx` and a matching source entry file.
Generated manifests import from `tspack/manifest`, so TS-aware editors provide autocomplete/typechecking while authoring.

## Kinds

- `--kind library` creates `src/index.ts` and a library-oriented manifest target.
- `--kind app` creates `src/main.ts` and an app-oriented manifest target with relaxed type policy defaults (`types: ""` for no public type output).

## Generated files

- `manifest.tsx`
- `src/index.ts` for libraries
- `src/main.ts` for apps
- `.tspack/types/tspack-manifest.d.ts` (local manifest authoring declaration surface)
- `tspack-env.d.ts` (project-level TypeScript reference for `tspack/manifest`)

## Flags

- `--kind <library|app>` (required)
- `--name <package-name>` (required)
- `--version <version>` (default: `0.1.0`)
- `--license <license>` (default: `MIT`)
- `--force` (overwrite generated files)
- `--dry-run` (print plan only)

## Publish policy note

The library template keeps publish contents explicit with `include={["dist/**", "README.md", "LICENSE"]}`. If you add a `CHANGELOG.md` file and want it in the package, add `"CHANGELOG.md"` to `<Publish include={...} />` explicitly. TSPack warns during pack when `CHANGELOG.md` exists but the final publish policy omits it; init does not add that include by default because it does not generate a changelog file.

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


## Manifest authoring type support

The generated declaration files exist only for local editor/autocomplete/typechecking support.
They are not runtime helpers and are not the manifest parser or validator source of truth.
If removed, regenerate them by rerunning `tspack init --force` in the project root.
