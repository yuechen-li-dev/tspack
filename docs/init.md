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
- `.tspack/types/tspack-xtest.d.ts` (local native xTest global declaration surface)
- `tsconfig.tspack.json` (editor/type support for TSPack-owned manifest and xTest TSX files)
- `tspack-env.d.ts` (project-level TypeScript reference for `tspack/manifest`)

## Flags

- `--kind <library|app>` (required)
- `--name <package-name>` (required)
- `--version <version>` (default: `0.1.0` for generated package manifests)
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
`tsconfig.tspack.json` maps `tspack/manifest` to `.tspack/types/tspack-manifest.d.ts`, isolates ambient packages with `"types": []`, includes TSPack-owned manifest files plus `src/*.xtest.tsx`, and uses `jsx: preserve` so React and `react/jsx-runtime` are not required for manifest or xTest editing. Ordinary app files such as `src/App.tsx` stay out because the config relies on a narrow include allowlist instead of a broad app-source glob.

Most projects should keep the generated helper as `TsConfig.manifestEditor()`.
Large compiler repos, monorepos with intentionally invalid fixtures, and repos
that keep template or testdata trees beside real manifests may opt into
`TsConfig.manifestEditor({ include, exclude })` to narrow the editor project
without hand-authoring raw JSON. When you override `include`, add
`.tspack/types/**/*.d.ts` yourself if you still want local manifest and xTest
declarations loaded.

In repos that do not already have a solution-style root TypeScript config,
editor support also needs a discoverable root `tsconfig.json` that references
`tsconfig.tspack.json`. That lets VS Code route `manifest.tsx` and included
`*.xtest.tsx` files into the manifest editor project instead of an inferred
React/app project. TSPack-owned example repos in this monorepo now follow that
pattern. For `init --alongside`, TSPack still does not overwrite an existing
user `tsconfig.json`; run `tspack compat write`, restart the TS server, and use
`TypeScript: Go to Project Configuration` if manifest files still look inferred.
If an existing `tsconfig.json` is present, init leaves it unchanged and prints guidance to exclude TSPack-owned files if the app config includes root TSX broadly.
If removed, regenerate these files by rerunning `tspack init --force` in the project root.

For `tspack init --alongside`, the root manifest is written first and the editor
support files remain explicit compat outputs. Run `tspack compat write` to
materialize `tsconfig.tspack.json`, `.vscode/settings.json`,
.vscode/extensions.json`, `.tspack/types/tspack-manifest.d.ts`, and
`.tspack/types/tspack-xtest.d.ts`. If VS Code
was already open, run `TypeScript: Restart TS Server` after the compat write.

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

# Optional validation before committing:
tspack check
tspack check --format
tspack update --policy --dry-run
```


React library starter:

```sh
tspack init --template react-library --name ui-kit --package @local/ui-kit
tspack update
tspack sync
tspack run typecheck
tspack run build
tspack run build-types
tspack pack --verify --package @local/ui-kit
```

For CLI-native documentation, run `tspack help init`, `tspack help workflow`, or `tspack help concepts`.
