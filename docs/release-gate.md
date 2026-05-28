# Release gate

## Smoke command checklist

### Core command surface

- `tspack init --kind <library|app> --name <package-name>`
- `tspack check`
- `tspack update`
- `tspack update --dry-run`
- `tspack sync`
- `tspack why <dep>`
- `tspack how --list`
- `tspack how TSPACK_IR_INVALID_RELATIVE_PATH`
- `tspack pack`
- `tspack test`
- `tspack artifact`
- `tspack bench`
- `tspack doom`
- `tspack run`
- `tspack inspect <target>` (**experimental**)
- `tspack format`
- `tspack lint`
- `tspack doctor`

### Claude-fooding Phase 2 package-manager smoke

The Phase 2 package-manager smoke must cover the validated update→store→sync loop and read-only UX commands:

- `tspack outdated --json`
- `tspack update --dry-run`
- `tspack update <declared-dep> --dry-run --json`
- `tspack update`
- `tspack sync`
- `tspack check --json`
- `tspack why <declared-dep>`
- `tspack how TSPACK_LOCK_VERSION_CONFLICT`

Fixture/fake-registry smoke should include:

- `tspack update --root <fixture>` followed by `tspack sync --root <fixture>`.
- `tspack update <declared-dep> --root <fixture>` preserving non-selected locked roots when valid.
- `tspack update <declared-dep> --root <fixture> --dry-run --json` with JSON-only stdout.
- `tspack outdated --root <fixture> --json` using metadata-only registry access.

### Claude-fooding Phase 3 boundary/import smoke

The Phase 3 boundary/import smoke must cover the remediated boundary model and its debugging tools:

- `tspack check --json` on a boundary fixture with structured diagnostics.
- `tspack check --explain src/file.ts` on a source file covered by boundary rules.
- `tspack how TSPACK_BOUNDARY_EXPLICIT_DENY`.
- `tspack how TSPACK_BOUNDARY_ALLOW_ONLY_VIOLATION`.
- `tspack how TSPACK_BOUNDARY_TYPE_EXPLICIT_DENY`.

Boundary/import test coverage should include:

- `.js` -> `.ts`/`.tsx` alias traversal.
- Workspace/path dependency matching by exact declared identity.
- `from` physical-file semantics versus `transitiveFrom` graph-reachable semantics.
- Runtime `allowOnly` enforcement.
- Type-level `denyTypeDeps` enforcement.
- Multiple boundary diagnostic types reported in one run.

## Mutation expectations

- `outdated`: no lock/store/node_modules mutation.
- `update --dry-run`: no lock/store/node_modules mutation.
- `update --dry-run --json`: no lock/store/node_modules mutation and stdout must remain JSON only.
- `update`: may write `ts-lock.toml` and populate the content-addressed store; must not create `node_modules`.
- `sync`: may materialize `node_modules`; must not mutate `ts-lock.toml`.
- `check`, `why`, and `how`: no lock/store/node_modules mutation.
- `pack`: may write package archives; must not mutate manifest/lock contract state.
- `run` and `inspect --run`: do not mutate manifest contract files and must not infer `package.json` scripts when no declared `RunTargets` exist.
- `artifact`, `test`, `bench`, and `doom`: may write harness outputs/artifacts but do not rewrite manifest/lock contract state.
- `format` and `lint`: are Biome-backed lifecycle UX commands; see `docs/format-lint.md` for file-writing behavior.
- `doctor`: is a non-mutating environment diagnostic command; see `docs/doctor.md`.

## Output expectations

- Text-mode `tspack update` writes plain progress/status lines to stderr, including resolve, store population/fetch, lockfile write, and completion phases.
- Text-mode `tspack update --dry-run` writes planning progress to stderr and does not include mutation phases.
- Targeted update output includes the selected dependency context.
- JSON modes keep stdout machine-readable; progress is suppressed or kept off stdout.
- `--quiet` suppresses update progress/status lines while leaving diagnostics and errors on stderr.

## Non-goal checks

- Unsupported command examples (`build`, `dev`, `publish`, `install`) must fail deterministically.
- `run` and `inspect` must not infer `package.json` scripts when no declared `RunTargets` exist.
- Lifecycle scripts and npm/npx compatibility mode remain out of scope.

## Manifest frontend build scope

- `npm run build` in `manifest-frontend/` validates production source files only (`src/index`, `src/cli`, and `src/inspect/*`).
- `npm test` in `manifest-frontend/` remains responsible for executing frontend tests.
- `npm run typecheck:manifest-api` in `manifest-frontend/` validates `tspack/manifest` authoring declarations against typed fixtures.
- Stricter standalone test-file typecheck is tracked as future M31c work.
