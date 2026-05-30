# `tspack migrate`

`tspack migrate` is an onboarding draft generator for existing npm-style projects. It reads the current root `package.json` and produces a reviewable TSPack draft, not a completed semantic conversion.

`tspack migrate` intentionally assumes that a human or LLM reviews the result. Existing npm projects often mix package metadata, build scripts, framework conventions, and dependency intent in ways that cannot be safely inferred from `package.json` alone.

## Default safety behavior

By default, `tspack migrate` is a dry-run/preview:

```sh
tspack migrate
```

The command prints:

- the `package.json` path it will read
- the manifest draft path it would write
- the migration report path it would write
- a summary of package kind, dependencies, targets, and TODOs

It writes no files unless `--write` is passed.

## Writing output files

```sh
tspack migrate --write
```

The default outputs are:

- `manifest.migrated.tsx`
- `tspack-migration.md`

`tspack migrate` never writes `manifest.tsx` by default and does not mutate `package.json`.

Without `--force`, any existing output file causes the command to fail before writing anything. This all-or-nothing behavior means that if `manifest.migrated.tsx` exists and `tspack-migration.md` does not, neither output is written. Pass `--force` to overwrite only the migration output paths.

## Flags

| Flag | Meaning |
|---|---|
| `--write` | Create output files. Omitted means preview only. |
| `--root <root>` | Set the project root. Defaults to the current directory. |
| `--package-json <path>` | Read an explicit package.json path. Relative paths are resolved from `--root`. |
| `--package-lock <path>` | Read an explicit npm package-lock path for migration report evidence. Relative paths are resolved from `--root`. |
| `--no-lock-evidence` | Skip package-lock evidence even when `package-lock.json` exists. |
| `--out-manifest <path>` | Write the manifest draft to a custom path. Relative paths are resolved from `--root`. |
| `--out-report <path>` | Write the migration report to a custom path. Relative paths are resolved from `--root`. |
| `--force` | Overwrite migration output files. Does not permit overwriting `manifest.tsx` unless that exact custom output path is explicitly requested. |

## Generated manifest draft

The manifest draft uses the current `tspack/manifest` authoring API and includes:

- package metadata from `name`, `version`, and `license`
- package kind inferred as `library` by default
- package kind inferred as `app` when `private: true` and no `exports`, `main`, `module`, or types field exists
- dependency declarations from `dependencies`, `peerDependencies`, `peerDependenciesMeta`, `optionalDependencies`, and `devDependencies`
- minimal target rows inferred from `exports`, `main`, `module`, `types`, and `typings`
- publish includes from `files`, or conservative defaults when `files` is absent
- comments with stable `MIGRATION_TODO_*` tags for every important uncertainty

The draft is meant to be readable TypeScript/TSX. Source entry paths are guesses, usually mapping `dist/index.js` to `src/index.ts`; they are marked with TODO comments because package.json generally does not declare source entry files.


## Package-lock evidence

By default, `tspack migrate` looks for `<root>/package-lock.json`. When present, it parses npm lockfile v2/v3 fields enough to enrich `tspack-migration.md` with review evidence. This evidence is intentionally report-only:

- no `ts-lock.toml` is generated from `package-lock.json`
- package-lock data is never treated as TSPack resolved truth
- package-lock/package.json files are not mutated
- `npm install`, dependency resolution, vulnerability scans, malware scans, and package scripts are not run

The lock evidence section reports:

- lockfile path, lockfileVersion, and package count
- direct package.json dependencies with declared range, locked version, integrity prefix, lock path, and resolved URL when present
- approximate transitive fanout from lock dependency names
- lifecycle script capabilities such as `preinstall`, `install`, `postinstall`, `prepare`, `prepack`, and publish-related hooks
- packages with `bin` fields
- locked peer dependency metadata
- likely platform/native packages from `os`, `cpu`, package-name markers, and known `@esbuild/*`, `@rollup/rollup-*`, and `@biomejs/cli-*` patterns
- mismatches such as missing direct lock entries, root lock declarations not represented in package.json fields consumed by migrate, duplicate versions, and `@types/*` evidence

Lifecycle scripts from the lockfile add `MIGRATION_TODO_SECURITY` report guidance. Binary packages, lifecycle capabilities, and large approximate fanout add dependency-classification review guidance. Detailed lock evidence stays in `tspack-migration.md`; the manifest draft remains package.json-driven.

If no implicit `<root>/package-lock.json` exists, migration continues and the report says lock evidence was not found. If the implicit lock exists but cannot be parsed, migration continues with a warning diagnostic and the report says lock evidence was invalid and ignored. If `--package-lock <path>` is provided, missing or invalid lock files fail the command because the user explicitly requested that evidence. `--package-lock` and `--no-lock-evidence` cannot be combined.

## TODO taxonomy

Stable TODO tags are emitted in both the manifest and report:

| TODO tag | Review prompt |
|---|---|
| `MIGRATION_TODO_TARGETS` | Verify target names, exports, source entries, runtime outputs, and declaration outputs. |
| `MIGRATION_TODO_DEP_CLASSIFICATION` | Verify dependency kind and whether dependencies should be scoped to specific targets. |
| `MIGRATION_TODO_RUN_TARGETS` | Review scripts manually. RunTargets describe runtime processes, not arbitrary npm scripts. |
| `MIGRATION_TODO_PUBLISH` | Verify publish contents with `tspack pack --dry-run`. |
| `MIGRATION_TODO_BOUNDARIES` | Replace strict defaults with project-specific boundary policy where needed. |
| `MIGRATION_TODO_TYPES` | Verify declaration and public type leakage policy. |
| `MIGRATION_TODO_SECURITY` | Review lifecycle scripts and dependency executable capabilities before acknowledging any capability. |

## Mapping rules

### Dependencies

- `peerDependencies` become `peer(npm(name, range))`.
- Optional peer metadata (`peerDependenciesMeta[name].optional == true`) becomes `peer(npm(name, range), { optional: true })`.
- `dependencies` become `dep(npm(name, range))` unless the same package is also a peer; peer classification wins and the duplicate is reported.
- `optionalDependencies` become `dep(npm(name, range))` with a TODO because optional runtime semantics require review.
- `devDependencies` become `tool(npm(name, range))`.
- Known tooling packages such as `typescript`, `vite`, `vitest`, `tsup`, `rollup`, `webpack`, `esbuild`, `@biomejs/biome`, `eslint`, `prettier`, `jest`, `playwright`, `@playwright/test`, `turbo`, and `nx` are classified as tools without an entry-specific TODO.
- Unknown dev dependencies are still emitted as tools with `MIGRATION_TODO_DEP_CLASSIFICATION`.

Dependency keys are generated as valid deterministic TypeScript identifiers. For example:

- `react-dom` -> `reactDom`
- `@types/node` -> `typesNode`
- `@scope/pkg` -> `scopePkg`

Identifier collisions are resolved with numeric suffixes and reported.

### Targets

- A simple `exports["."]` object or string becomes a `core` target.
- `main`, `module`, `types`, and `typings` become a `core` target when no root export is available.
- Simple subpath exports may become additional target rows such as `./react` -> `react`.
- Complex conditional exports are reported for review instead of over-inferred.
- Missing runtime/types metadata falls back to conservative placeholders so the draft remains editable and reviewable.

### Publish

- If `package.json.files` exists, the generated `<Publish include>` uses those entries exactly in package.json order.
- If `files` is absent, the draft uses `dist/**`, `README.md`, and `LICENSE` as a conservative starting point.
- `CHANGELOG.md` is not added automatically unless package.json already lists it.

### Scripts and security

Scripts are listed in `tspack-migration.md` but are not migrated to RunTargets and are never executed. Lifecycle script names such as `preinstall`, `install`, `postinstall`, and `prepare` are reported under security with `MIGRATION_TODO_SECURITY`.

## Report contents

`tspack-migration.md` includes:

- inputs and output paths
- package-lock evidence status and report-only lock evidence when available
- package summary and inferred kind
- dependency counts and target count
- mechanical mapping table
- dependency rows
- duplicate peer/runtime declarations and identifier collisions when present
- grouped TODO sections
- scripts not migrated
- lifecycle, binary, peer, platform/native, duplicate-version, and mismatch evidence from package-lock when available
- suggested next steps

The report does not claim migration is complete.

## Examples

Preview only:

```sh
tspack migrate --root packages/components
```

Write the default draft and report:

```sh
tspack migrate --root packages/components --write
```

Write to custom paths:

```sh
tspack migrate \
  --package-json ./package.json \
  --out-manifest ./tmp/manifest.review.tsx \
  --out-report ./tmp/tspack-migration.md \
  --write
```

Overwrite previous migration outputs:

```sh
tspack migrate --write --force
```

## Non-goals

`tspack migrate` does not:

- translate `package-lock.json` to `ts-lock.toml`
- translate package-lock graphs into TSPack lock data
- scan source imports
- infer target boundaries from source code
- overwrite `manifest.tsx` automatically
- mutate `package.json`
- install dependencies
- run `tspack update`, `tspack sync`, `tspack check`, `tspack pack`, or package scripts
- call LLM APIs
- provide framework adapters
- perform full monorepo workspace discovery beyond the current package.json root
- analyze source maps
