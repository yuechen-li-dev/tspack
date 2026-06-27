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

## Template pipeline

Template TOML and files are input syntax, not the internal semantic model. The loader first parses metadata into a raw template shape, normalizes it into an internal TemplateIR, then lowers that IR into a concrete TemplatePlan for a specific `tspack init` invocation before any file is written.

This pipeline is internal and behavior-preserving for existing templates. It keeps built-in and local templates on the same path, makes planning dry-run-able, and keeps safety checks centralized around declared file projections. Concepts are metadata today; future template composition work should happen at the IR layer without treating overlays as implemented in v0.1.3. M60a records a future concept-fragment composition design in `docs/design/concept-fragment-composition.md`; concept fragments are not implemented behavior.

## Built-in static template

The built-in `static` template is stored in the repo and loaded through the public template engine. It creates a minimal TypeScript browser app with `manifest.tsx`, `package.json`, `tsconfig.tspack.json`, `biome.json`, `index.html`, `src/main.ts`, `src/style.css`, local manifest types, and a concise README. Because it emits `biome.json`, its manifest declares `@biomejs/biome` as a tool dependency so `tspack check --format` can use the project-materialized backend after `tspack update` and `tspack sync`.

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

The manifest declares `react` and `react-dom` as runtime dependencies, Vite/TypeScript/plugin/type/Biome packages as tools, Node-backed Vite run targets (`dev`, `build`, and `preview`), manual React runtime update policy, rolling minor tooling update policy, and consumer-install plus maintainer-publish lifecycle category acknowledgments. Acknowledgments keep known tool-closure lifecycle scripts auditable and do not allow execution; TSPack still blocks lifecycle execution by default. `package.json` is compatibility glue only and contains no lifecycle scripts. Run `tspack update` and `tspack sync` before `tspack check --format`, `tspack run dev`, or `tspack run build`; sync materializes the project `node_modules/.bin` shims that let TSPack launch Vite without a global install.


## React library template

`react-library` generates a React + Vite + TypeScript component library starter:

```sh
tspack init --template react-library --name ui-kit --package @local/ui-kit
```

Concepts: `tspack.workspace`, `tspack.manifestBoundary`, `tspack.securityPolicy`, `tspack.updatePolicy`, `tspack.pack`, `typescript.library`, `vite.library`, `react.library`, `package.exports`, `package.peerDependencies`, and `browser.components`.

Generated files include `manifest.tsx`, `tsconfig.tspack.json`, `tsconfig.json`, `tsconfig.build.json`, `biome.json`, `package.json`, `vite.config.ts`, `src/index.ts`, `src/Button.tsx`, `src/style.css`, and `README.md`. The template deliberately does not create an app `index.html`; it is a reusable component library shape rather than a SPA.

React and React DOM are modeled as peer dependencies in the manifest and compatibility `package.json`. Tooling dependencies, including `@biomejs/biome`, remain tools. The compatibility `package.json` includes module/type export metadata and is marked `private` to avoid accidental npm publication. TSPack pack flows remain the intended publication path. Run `tspack update` and `tspack sync` before `tspack check --format`, `tspack run typecheck`, `tspack run build`, or `tspack run build-types` so the project tool shims for Biome, Vite, and `tsc` are materialized.

Run targets are intentionally simple and Node-oriented: `build` runs Vite library mode, `build-types` emits declarations with `tsc -p tsconfig.build.json`, and `typecheck` runs `tsc -p tsconfig.json --noEmit`. TSPack does not sequence those targets yet, so run both `build` and `build-types` before pack verification.

The editor boundary mirrors the other built-in templates: `tsconfig.tspack.json` covers TSPack-owned manifest files without requiring the React JSX runtime, while the library `tsconfig.json` uses `jsx: "react-jsx"` and excludes TSPack-owned files.

## Experimental local custom concept fragments

Local templates may opt into experimental local concept fragments. They are inert TOML files declared by the template; they do not run scripts, execute commands, fetch remote content, install packages, or invoke other templates.

Template metadata uses an explicit `[[localConcepts]]` table so it does not conflict with the existing `concepts = [...]` stack:

```toml
format = 1
name = "my-company-react"
description = "Company React starter"
kind = "app"
concepts = ["react.app", "browser.spa", "typescript.app", "my-company.design-system"]

[[localConcepts]]
name = "my-company.design-system"
path = "concepts/design-system.toml"
```

The local concept file format is also TOML:

```toml
format = 1
name = "my-company.design-system"
description = "Company design system additions"
provides = ["my-company.design-system"]
expects = ["react.app"]
conflicts = []
compatibleKinds = ["app"]

[[files]]
destination = "src/design-system.ts"
source = "files/design-system.ts.tmpl"
render = true
```

M60f supports concept identity constraints (`provides`, `expects`, `expectsAnyOf`, `conflicts`, and `compatibleKinds`) plus file contributions. File `source` paths are relative to the concept file directory. File `destination` paths are relative to the generated project root. Both are validated as safe relative paths with no absolute paths, no `..` traversal, no Windows drive prefixes, and no remote URLs. Rendered concept files use the same `{{variable}}` placeholder syntax and value environment as ordinary template files.

Local concepts are loaded into the same concept registry as built-ins for that template's planning run. The explicit template `concepts = [...]` list remains the source of truth: TSPack does not auto-insert missing concepts, listed order remains priority order, missing expectations fail, conflicts fail deterministically, duplicate local names fail, and local concepts may not shadow built-in concept names.

Local templates can opt into generic concept-rendered manifest generation:

```toml
[generation]
manifest = "concept"
```

When this mode is present, TSPack plans `manifest.tsx` from the explicit concept stack and merged concept IR. The template must not also project a normal `manifest.tsx`; that is a semantic conflict and `--force` does not bypass it. Non-manifest template files and safe local concept file contributions still work normally. Concept-rendered manifest mode only creates the manifest contract; the template must still project the normal source and config files needed by that contract, such as `package.json`, `tsconfig.json`, `vite.config.ts`, `index.html`, and `src/main.tsx`, unless a supported concept file contribution provides them.

The generic local renderer currently supports a single workspace/package manifest with template defaults (`projectName`, `packageName`, `runtime`, version `0.1.0`, and template kind), dependencies, tools, peers, targets, target dependency wiring, run targets, update policy rows, security lifecycle category acknowledgements, manifest boundary policies suitable for app templates, and concept metadata comments. Local concept TOML can declare:

```toml
[[dependencies]]
key = "clsx"
source = "npm"
range = "^2.1.1"

[[runTargets]]
name = "generate-icons"
command = "node -e console.log('icons-generated')"
```

A check-clean React/Vite local template should list the built-in React, browser SPA, Vite, TypeScript, workspace, manifest-boundary, update-policy, and security-policy concepts, add any local concept dependency/run-target/file contributions, and project the ordinary Vite app files. The test fixture at `internal/templates/testdata/local-concepts/concept-manifest-app` is the minimal example: it uses a local design-system concept, renders `manifest.tsx` from concepts, contributes `src/design-system.ts`, and still projects `src/main.tsx` plus Vite/TypeScript config files.

If the `[generation]` table is absent, existing behavior remains: manifest-like local concept contributions fail with `TSPACK_TEMPLATE_CONCEPT_UNSUPPORTED_CONTRIBUTION` instead of being silently dropped. Unknown generation manifest modes fail with `TSPACK_TEMPLATE_INVALID`. Unsupported concept manifest fields in concept-rendered mode also fail loudly. Env rendering, service requirements, pack metadata, remote concepts, JavaScript/TypeScript concept files, scripts, package installation during init, arbitrary text patches, template inheritance, and public concept registries are not supported.
