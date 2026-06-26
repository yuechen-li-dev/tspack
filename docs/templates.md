# TSPack templates

A TSPack template declares project birth intent as inert data: metadata, concepts, variables, and file projections.

## Metadata file

Templates are directories containing `tspack-template.toml`:

```toml
format = 1
name = "static"
description = "Static TypeScript browser app"
kind = "app"
concepts = ["tspack.workspace", "typescript.app", "browser.static"]

[variables.projectName]
description = "Project display name"
default = "my-app"

[[files]]
from = "files/index.html.tmpl"
to = "index.html"
```

Required fields are `format`, `name`, `description`, `kind`, at least one `concepts` entry, and at least one `[[files]]` entry.

## Concepts

Concepts are metadata in M54a. They are parsed, syntax-validated, listed by `--list-templates`, and printed after init. Concept names may use dotted identifiers such as `tspack.workspace`, `tspack.manifestBoundary`, `typescript.app`, and `browser.static`. Custom templates may define custom concepts.

## Variables and placeholders

Variables can declare a description, default, and allowed values. Init supplies common variables from flags: `--name` maps to `projectName`, `--package` maps to `packageName`, and `--runtime` maps to `runtime`.

Only simple placeholders are supported:

```text
{{projectName}}
{{packageName}}
{{runtime}}
{{customVariable}}
```

There are no loops, conditionals, function calls, shell interpolation, environment expansion, or escaping in M54a. Unknown placeholders fail with `TSPACK_TEMPLATE_UNKNOWN_VARIABLE`; missing required variables fail with `TSPACK_TEMPLATE_VARIABLE_MISSING`.

## Files and safety

Only source paths ending in `.tmpl` are rendered. Other files are copied unchanged. All `from` and `to` paths must be relative, cannot use `..`, and cannot escape the template root or destination root. Existing target files fail with `TSPACK_TEMPLATE_FILE_EXISTS` unless `--force` is used. `--force` overwrites only declared template files and does not delete unrelated files.

## Built-in static template

The built-in `static` template is stored in the repo and loaded through the public template engine. It creates a minimal TypeScript browser app with `manifest.tsx`, `package.json`, `tsconfig.tspack.json`, `biome.json`, `index.html`, `src/main.ts`, `src/style.css`, local manifest types, and a concise README.

## Local templates

Run a local template with:

```sh
tspack init --template ./path/to/template --name demo
```

The path must point to a directory containing `tspack-template.toml`.

## Built-in `react` template

The built-in `react` template generates a plain React + Vite + TypeScript browser app through the same inert template engine used by local templates and the built-in `static` template.

```sh
tspack init --template react --name my-app
```

Concepts:

- `tspack.workspace`
- `tspack.manifestBoundary`
- `tspack.securityPolicy`
- `tspack.updatePolicy`
- `typescript.app`
- `vite.app`
- `react.app`
- `browser.spa`

Variables:

- `projectName` from `--name`
- `packageName` from `--package`, defaulting to `projectName`
- `runtime` from `--runtime`, allowed values `nodejs`, `bun`, and `deno`

Generated files include `manifest.tsx`, `tsconfig.tspack.json`, `tsconfig.json`, `biome.json`, `vite.config.ts`, `package.json`, `index.html`, `src/main.tsx`, `src/App.tsx`, `src/style.css`, and `README.md`. The app tsconfig uses React JSX and excludes TSPack manifest/xTest files, while `tsconfig.tspack.json` preserves JSX for manifest editing and maps `tspack/manifest` to generated local declarations.

The manifest declares `react` and `react-dom` as runtime dependencies, Vite/TypeScript/plugin/type packages as tools, Node-backed Vite run targets (`dev`, `build`, and `preview`), manual React runtime update policy, rolling minor tooling update policy, and a maintainer-publish lifecycle category acknowledgment. `package.json` is compatibility glue only and contains no lifecycle scripts.


## React library template

`react-library` generates a React + Vite + TypeScript component library starter:

```sh
tspack init --template react-library --name ui-kit --package @local/ui-kit
```

Concepts: `tspack.workspace`, `tspack.manifestBoundary`, `tspack.securityPolicy`, `tspack.updatePolicy`, `tspack.pack`, `typescript.library`, `vite.library`, `react.library`, `package.exports`, `package.peerDependencies`, and `browser.components`.

Generated files include `manifest.tsx`, `tsconfig.tspack.json`, `tsconfig.json`, `tsconfig.build.json`, `biome.json`, `package.json`, `vite.config.ts`, `src/index.ts`, `src/Button.tsx`, `src/style.css`, and `README.md`. The template deliberately does not create an app `index.html`; it is a reusable component library shape rather than a SPA.

React and React DOM are modeled as peer dependencies in the manifest and compatibility `package.json`. Tooling dependencies remain tools. The compatibility `package.json` includes module/type export metadata and is marked `private` to avoid accidental npm publication. TSPack pack flows remain the intended publication path.

Run targets are intentionally simple and Node-oriented: `build` runs Vite library mode, `build-types` emits declarations with `tsc -p tsconfig.build.json`, and `typecheck` runs `tsc -p tsconfig.json --noEmit`. TSPack does not sequence those targets yet, so run both `build` and `build-types` before pack verification.

The editor boundary mirrors the other built-in templates: `tsconfig.tspack.json` covers TSPack-owned manifest files without requiring the React JSX runtime, while the library `tsconfig.json` uses `jsx: "react-jsx"` and excludes TSPack-owned files.
