# `tspack init` (M30)

`tspack init` scaffolds a readable starter `manifest.tsx` and a matching source entry file.
Generated manifests import from `tspack/manifest`, so TS-aware editors provide autocomplete/typechecking while authoring. Init also generates `tsconfig.tspack.json` so manifest TSX is checked as TSPack DSL instead of React TSX.

## Kinds

- `--kind library` creates `src/index.ts` and a library-oriented manifest target.
- `--kind app` creates `src/main.ts` and an app-oriented manifest target with relaxed type policy defaults (`types: ""` for no public type output).

## Generated files

- `manifest.tsx`
- `src/index.ts` for libraries
- `src/main.ts` for apps
- `.tspack/types/tspack-manifest.d.ts` (local manifest authoring declaration surface)
- `tsconfig.tspack.json` (editor/type support for TSPack-owned manifest and xTest TSX files)
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

The generated declaration files and `tsconfig.tspack.json` exist only for local editor/autocomplete/typechecking support.
They are not runtime helpers and are not the manifest parser or validator source of truth.
`tsconfig.tspack.json` maps `tspack/manifest` to `.tspack/types/tspack-manifest.d.ts`, includes TSPack-owned manifest and `*.xtest.tsx` files, excludes app source, and uses `jsx: preserve` so React and `react/jsx-runtime` are not required for manifest editing.
If an existing `tsconfig.json` is present, init leaves it unchanged and prints guidance to exclude TSPack-owned files if the app config includes root TSX broadly.
If removed, regenerate these files by rerunning `tspack init --force` in the project root.

## Template engine (M54a)

By default, `tspack init` now renders the built-in `static` template through the same inert template engine used by local templates. Use `--template <name-or-path>` to select a built-in template name or a local directory containing `tspack-template.toml`.

Template flags:

- `--template <name-or-path>` selects a built-in template or local template directory.
- `--list-templates` prints built-in templates with kind, description, and concepts.
- `--name <projectName>` sets the template `projectName` variable; otherwise the current directory name is used.
- `--package <packageName>` sets the template `packageName` variable; otherwise it follows `projectName`.
- `--runtime <nodejs|bun|deno>` sets the workspace runtime variable when the template declares it.
- `--force` overwrites only files declared by the selected template.

Templates are data only: they copy files, create parent directories, and replace `{{variableName}}` placeholders in `.tmpl` inputs. Templates cannot run shell commands, install packages, fetch remote code, execute lifecycle scripts, or delete arbitrary files.

Successful template init prints the selected template concepts and next steps:

```sh
tspack update
tspack sync
tspack run dev
tspack check
```

## React app template

`tspack init` still defaults to the built-in `static` template. Use the explicit `react` template for a practical React + Vite + TypeScript app:

```sh
tspack init --template react --name my-app
```

The generated project keeps `manifest.tsx` as the source of truth and writes `package.json`, `tsconfig.json`, `tsconfig.tspack.json`, `vite.config.ts`, `index.html`, and `src/**` as compatibility/tooling files. Next steps:

```sh
tspack update
tspack sync
tspack run dev
tspack check
tspack check --format
tspack update --policy --dry-run
```
