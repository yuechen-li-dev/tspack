# `tspack migrate`

`tspack migrate` is an onboarding draft generator for existing npm-style projects. It reads the current root `package.json` and produces a reviewable TSPack draft, not a completed semantic conversion.

`tspack migrate` intentionally assumes that a human or LLM reviews the result. Existing npm projects often mix package metadata, build scripts, framework conventions, and dependency intent in ways that cannot be safely inferred from `package.json` alone.

For the migration track closeout, thesis, TODO taxonomy, and golden review workflow, see [Migration Closeout](claude-fooding-migration.md).

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

After validation and review, promote the draft to the root `manifest.tsx` before running lifecycle commands. The draft filename is deliberately inert: `check`, `update`, `sync`, and `pack` require the authoritative root manifest rather than accepting `manifest.migrated.tsx` as project truth.

Without `--force`, any existing output file causes the command to fail before writing anything. This all-or-nothing behavior means that if `manifest.migrated.tsx` exists and `tspack-migration.md` does not, neither output is written. Pass `--force` to overwrite only the migration output paths.

## Flags

| Flag | Meaning |
|---|---|
| `--write` | Create output files. Omitted means preview only. |
| `--check` | Validate the generated draft with the manifest frontend, Go manifest IR validation, and TODO accounting. Dry-run check writes nothing. With `--write`, validation errors block output writes. |
| `--root <root>` | Set the project root. Defaults to the current directory. |
| `--package-json <path>` | Read an explicit package.json path. Relative paths are resolved from `--root`. |
| `--package-lock <path>` | Read an explicit npm package-lock path for migration report evidence. Relative paths are resolved from `--root`. |
| `--no-lock-evidence` | Skip package-lock evidence even when `package-lock.json` exists. |
| `--scan-source` | Enable source import evidence scanning. This is the default for conservative source roots, and the flag is accepted for explicitness. |
| `--no-source-scan` | Skip source import evidence scanning. |
| `--out-manifest <path>` | Write the manifest draft to a custom path. Relative paths are resolved from `--root`. |
| `--out-report <path>` | Write the migration report to a custom path. Relative paths are resolved from `--root`. |
| `--force` | Overwrite migration output files. Does not permit overwriting `manifest.tsx` unless that exact custom output path is explicitly requested. |

## Validation loop

```sh
tspack migrate --check
```

`--check` validates the generated migration draft without running update/sync, resolving packages, installing dependencies, materializing `node_modules`, creating `ts-lock.toml`, mutating source/package files, or executing package scripts. In dry-run mode it writes the draft to a temporary file outside the project, validates that temporary draft, cleans it up, and leaves the project tree unchanged.

Validation is layered:

1. **Manifest frontend validation** parses the generated TSX with the current `tspack/manifest` authoring subset. This catches syntax, unsupported helpers/components, forbidden dynamic expressions, and generated API incompatibilities.
2. **Go manifest IR validation** validates the manifest shape emitted by the frontend, including workspace/package metadata, dependency intent, targets, publish policy, security shape, and policy shape. It intentionally does not validate or require a lockfile.
3. **TODO accounting** counts remaining `MIGRATION_TODO_*` groups and reports them. TODOs are review work, not validation errors.

If validation fails, the diagnostics distinguish structural frontend/IR failures from TODO accounting. `remainingTodos` reports review work still present, and `todosAreErrors: false` makes clear that `MIGRATION_TODO_*` comments did not fail validation by themselves. Unknown dependency-reference failures may also include an alias/key mismatch hint when generated refs do not match declared dependency identities.

A passed check means the draft is structurally valid and accepted by the current manifest frontend and Go IR validator. It does **not** mean the migration is semantically complete, dependencies are resolved, targets are correct, scripts are migrated, or publish contents are verified.

After resolving the draft TODOs, promote it to `manifest.tsx`, then run `tspack update` and `tspack check`. Do not pass the draft filename to lifecycle commands; those commands intentionally require an authoritative root manifest.

With `--write --check`, TSPack validates first. If validation fails, no migration outputs are written. If validation passes, it writes the manifest draft and migration report. Existing output collisions are still checked before validation unless `--force` is passed.

Examples:

```sh
tspack migrate --check
tspack migrate --write --check
tspack migrate --check --no-lock-evidence --no-source-scan
tspack migrate --check --out-manifest ./tmp/manifest.review.tsx --out-report ./tmp/tspack-migration.md
```

## Generated manifest draft

The manifest draft uses the current `tspack/manifest` authoring API and includes:

- package metadata from `name`, `version`, and `license`
- package kind inferred as `library` by default
- package kind inferred as `app` when `private: true` and no `exports`, `main`, `module`, or types field exists
- dependency declarations from `dependencies`, `peerDependencies`, `peerDependenciesMeta`, `optionalDependencies`, and `devDependencies`
- minimal target rows inferred from `exports`, `main`, `module`, `types`, and `typings`
- publish includes from `files`, or conservative defaults when `files` is absent
- comments with stable `MIGRATION_TODO_*` tags for every important uncertainty

The generated workspace intentionally omits `runtime="nodejs"` because Node.js is the default runtime profile. M42b does not infer Bun or Deno from `packageManager` or other package-manager fields; future runtime clues can be reviewed explicitly without changing migration's current behavior-preserving baseline.

The draft is meant to be readable TypeScript/TSX. Source entry paths are guesses, usually mapping `dist/index.js` to `src/index.ts`; they are marked with TODO comments because package.json generally does not declare source entry files. Generated `runtime` and `types` paths omit leading `./` segments, so package fields such as `"./dist/index.js"` become `"dist/index.js"`; parent paths such as `"../dist/index.js"` are preserved.

Generated target rows use string dependency identity references for `deps` and `peers`, for example `deps: ["react-dom"]` and `peers: ["@types/react"]`. Tool rows continue to pass dependency objects to `<Tools values={[deps.typescript]} />`, because tool values are declared dependency intents.

## Script classification and RunTarget suggestions

`tspack migrate` classifies `package.json` scripts as migration evidence, but it does not run them and does not convert them into active manifest rows. The generated manifest only receives a `MIGRATION_TODO_RUN_TARGETS` comment. Detailed script evidence and any suggested runtime targets are written to `tspack-migration.md`.

RunTargets are declared runtime processes. They are not a compatibility layer for arbitrary npm script soup. This means:

- dev-server/runtime scripts such as `dev: vite`, `dev: next dev`, `storybook: storybook dev -p 6006`, `serve: vite preview`, or `start: node server.js` may be reported as RunTarget candidates
- build scripts such as `vite build`, `tsc`, `tsup`, `rollup -c`, or `next build` stay as report evidence
- test scripts such as `vitest`, `jest`, `playwright test`, or `node --test` stay as report evidence
- lint/format scripts such as `eslint .`, `biome lint .`, `prettier --write .`, or `biome format --write .` stay as report evidence
- package/release scripts such as `pack`, `prepack`, `prepublishOnly`, `release`, or `changeset` stay as report evidence
- lifecycle scripts such as `preinstall`, `install`, `postinstall`, `prepare`, `prepack`, and publish hooks remain security-relevant executable capabilities and are not executed

The report includes a `## Scripts and RunTarget suggestions` section with:

- a complete scripts-not-migrated table containing script name, category, command, and suggested action
- a RunTarget candidates table for likely long-running runtime/dev-server scripts only
- a non-RunTarget scripts list for build/test/lint/format/package/maintenance scripts
- a shell/env review list for commands that need manual handling before any target is enabled

For simple commands, migrate records a best-effort argv suggestion. Example report row:

```md
| dev | dev | high | ["vite", "--host", "127.0.0.1"] | http://127.0.0.1:5173 | http / | verify cwd/url |
```

Known default readiness hints are conservative:

- Vite dev/preview defaults to `http://127.0.0.1:5173` with HTTP `/` readiness
- Next dev defaults to `http://127.0.0.1:3000` with HTTP `/` readiness
- Storybook defaults to `http://127.0.0.1:6006` with HTTP `/` readiness
- `--port` and `-p` are used when easy to read from a simple argv
- commands such as `node server.js` get command argv evidence but keep readiness as a TODO

Shell features intentionally stop short of conversion. Commands containing `&&`, `||`, `;`, redirection, pipes, command substitution, or backticks are marked for review. Environment prefixes such as `PORT=3000 vite` and `cross-env PORT=3000 vite` are also marked for review; M41d does not add manifest env fields.

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

## Source import evidence

By default, `tspack migrate` scans common source roots when they exist: `src`, `lib`, and `app`. If `package.json` has a string `source` field under the project root, that path is also scanned. The scan is read-only and is used only as migration evidence. Pass `--no-source-scan` to skip this work.

The source scan recognizes simple static imports, re-exports, `import("pkg")`, and `require("pkg")` string literals in `.ts`, `.tsx`, `.js`, `.jsx`, `.mts`, `.cts`, `.mjs`, and `.cjs` files. It maps subpath imports such as `react/jsx-runtime` to `react` and `@scope/pkg/subpath` to `@scope/pkg`. Relative imports are ignored for dependency classification. Node builtins such as `node:fs` and `path` are reported separately and never converted to npm dependencies.

The report includes:

- scan status, roots, file counts, warnings, and truncation status
- observed external packages with runtime/type-only/mixed/dynamic evidence
- package.json declaration status for each observed package
- runtime imports declared only in `devDependencies`
- type-only-only import candidates for `MIGRATION_TODO_TYPES` review
- imported packages missing from direct package.json dependency fields
- declared packages not observed in scanned source
- Node builtin evidence for runtime/environment review

The manifest stays conservative. Source evidence may add short comments and `MIGRATION_TODO_DEP_CLASSIFICATION` / `MIGRATION_TODO_TYPES` notes, but migrate does not move dependencies between fields, remove unobserved dependencies, infer exact target boundaries, or mutate source/package files based on the scan.

Source scanning uses conservative guardrails: at most 2,000 files and at most 1 MiB per file. If evidence is incomplete, migration continues and the report/diagnostics mark the scan as truncated or warn about unreadable files.

Limitations: the scan does not run TypeScript semantic checking, perform full module resolution, resolve `tsconfig` path aliases, analyze bundler/framework configuration, execute scripts, install packages, or call LLMs. Human/LLM review should use the evidence to resolve `MIGRATION_TODO_DEP_CLASSIFICATION`, `MIGRATION_TODO_TYPES`, and `MIGRATION_TODO_TARGETS` rather than treating it as architectural truth.

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
- Optional peer metadata (`peerDependenciesMeta[name].optional == true`) becomes `peer(npm(name, range), { optional: true })`, or `peer(npm(name, range), { key: name, optional: true })` when the generated TypeScript identifier differs from the package identity.
- `dependencies` become `dep(npm(name, range))` unless the same package is also a peer; peer classification wins and the duplicate is reported.
- `optionalDependencies` become `dep(npm(name, range))` with a TODO because optional runtime semantics require review.
- `devDependencies` become `tool(npm(name, range))`.
- Known tooling packages such as `typescript`, `vite`, `vitest`, `tsup`, `rollup`, `webpack`, `esbuild`, `@biomejs/biome`, `eslint`, `prettier`, `jest`, `playwright`, `@playwright/test`, `turbo`, and `nx` are classified as tools without an entry-specific TODO.
- Unknown dev dependencies are still emitted as tools with `MIGRATION_TODO_DEP_CLASSIFICATION`.

Dependency properties are generated as valid deterministic TypeScript identifiers. Whenever the generated identifier differs from the npm package identity, migrate emits an explicit dependency `key` so generated refs preserve the package identity:

```tsx
const deps = defineDeps({
  biomejsBiome: tool(npm("@biomejs/biome", "^1.9.4"), {
    key: "@biomejs/biome",
  }),
  reactDom: peer(npm("react-dom", "^18.3.1"), {
    key: "react-dom",
  }),
});
```

Examples:

- `react-dom` -> `reactDom` with `key: "react-dom"`
- `@types/node` -> `typesNode` with `key: "@types/node"`
- `@scope/pkg` -> `scopePkg` with `key: "@scope/pkg"`
- `typescript` -> `typescript` with no explicit key, because the identifier already equals the package identity

Identifier collisions are resolved with numeric suffixes and reported. Target `deps` and `peers` generated by migrate use string package identities rather than `deps.<alias>` object references.

### Targets

- A simple `exports["."]` object or string becomes a `core` target.
- `main`, `module`, `types`, and `typings` become a `core` target when no root export is available.
- Simple subpath exports may become additional target rows such as `./react` -> `react`.
- Complex conditional exports are reported for review instead of over-inferred.
- Missing runtime/types metadata falls back to conservative placeholders so the draft remains editable and reviewable.
- Generated `runtime` and `types` values trim only leading `./` segments from package file paths; `../` and absolute paths are not rewritten.

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
- source import evidence status and report-only dependency classification hints when available
- package summary and inferred kind
- dependency counts and target count
- mechanical mapping table
- dependency rows
- duplicate peer/runtime declarations and identifier collisions when present
- grouped TODO sections
- scripts not migrated
- lifecycle, binary, peer, platform/native, duplicate-version, and mismatch evidence from package-lock when available
- validation status: not run by default, or passed/failed when `--check` is used
- suggested next steps

The report does not claim migration is complete.

## Examples

Preview and validate without writing:

```sh
tspack migrate --root packages/components --check
```

Preview only:

```sh
tspack migrate --root packages/components
```

Write the default draft and report after validation:

```sh
tspack migrate --root packages/components --write --check
```

Write the default draft and report without validation:

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
- mutate source files or package.json based on source scan evidence
- infer exact target boundaries from source file locations
- infer target boundaries from source code
- overwrite `manifest.tsx` automatically
- mutate `package.json`
- install dependencies
- run `tspack update`, `tspack sync`, `tspack check`, `tspack pack`, dependency resolution, package installation, materialization, or package scripts
- call LLM APIs
- provide framework adapters
- perform full monorepo workspace discovery beyond the current package.json root
- analyze source maps

## Related docs

- [Claude-Fooding Phase 8a Carryover](claude-fooding-phase8a-carryover.md)
