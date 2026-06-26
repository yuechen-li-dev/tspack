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
