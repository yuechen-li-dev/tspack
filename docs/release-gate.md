# Release gate

## Smoke command checklist

### Core command surface

- `tspack init --kind <library|app> --name <package-name>`
- `tspack check`
- `tspack update`
- `tspack update --dry-run`
- `tspack sync`
- `tspack why <dep>`
- `tspack why <dep> --json`
- `tspack why --reverse <lock-package-or-name>`
- `tspack why --reverse <lock-package-or-name> --json`
- `tspack how --list`
- `tspack how TSPACK_IR_INVALID_RELATIVE_PATH`
- `tspack pack`
- `tspack pack --verify`
- `tspack test`
- `tspack artifact`
- `tspack bench`
- `tspack doom`
- `tspack run`
- `tspack inspect <target>` (**experimental**)
- `tspack format`
- `tspack lint`
- `tspack doctor`


### Pack safety smoke

The pack release smoke should cover strict pack safety defaults:

- A workspace pack where one selected package is valid and another selected package fails pack validation must exit nonzero and leave no final `.tgz` artifacts.
- `tspack pack --package <valid-package>` in that workspace must still write the selected package artifact.
- A package with `publish.include = ["dist/**"]` and missing build output must fail with `TSPACK_PACK_INCLUDE_MATCHED_NOTHING` and write no artifact.
- `tspack pack --dry-run` must validate include patterns, fail when the real pack would fail, and write no artifacts.
- `tspack pack --verify` must verify produced archives before finalizing them, print verified-artifact output on success, and leave no final `.tgz` files when verification fails.
- `tspack pack --dry-run --verify` must fail deterministically with `TSPACK_PACK_INVALID_ARGS` and write no artifacts.
- A package with `CHANGELOG.md` at its root but no final publish-policy entry for it must warn with `TSPACK_PACK_CHANGELOG_NOT_INCLUDED` while still succeeding when no error diagnostics exist.

### Claude-fooding Phase 6 pack/why smoke

The Phase 6 pack/why smoke must cover the closeout state documented in `docs/claude-fooding-phase6.md`:

- `tspack pack --dry-run` validates the selected package set, prints the planned contents, and writes no artifacts.
- `tspack pack --verify` structurally verifies produced npm artifacts before finalization.
- `tspack pack --package <pkg> --verify` verifies one package selection without packing unrelated packages.
- Generated `package/package.json` metadata checks cover `license`, `main`, `types`, `peerDependencies`, and optional `peerDependenciesMeta`.
- All-or-nothing failure smoke verifies that a selected-package failure exits nonzero and leaves no partial final artifacts.
- Include matched nothing smoke verifies `TSPACK_PACK_INCLUDE_MATCHED_NOTHING` as an error.
- Changelog omission smoke verifies `CHANGELOG.md` omission warns with `TSPACK_PACK_CHANGELOG_NOT_INCLUDED` without auto-including or mutating publish policy.
- Deterministic repeated pack hash smoke verifies identical selected inputs produce the same reported `sha256:<hex>` hash.
- `tspack why <declared-dep>` explains declared dependency reachability.
- `tspack why <bare-transitive-name>` suggestion smoke verifies full lock-ID guidance for matching transitives.
- `tspack why npm:<name>@<version>` explains an exact lock package.
- `tspack why --json` emits deterministic structured output.
- `tspack why --reverse <name>` emits root-to-query reverse dependency paths.
- `tspack why <declared-dep> --package <pkg>` scopes explanations to the selected package.

Phase 6 expected behavior coverage should verify:

- Pack failures leave no partial artifacts from the selected package set.
- `pack --verify` does not run scripts, execute package code, install dependencies, publish, or perform registry/network checks.
- Why JSON keeps stdout clean for parseable JSON on handled paths.
- Reverse why paths are printed and encoded root-to-query.
- Lock edges in normal why are declaration-scoped and deduplicated.

### Claude-fooding Phase 2 package-manager smoke

The Phase 2 package-manager smoke must cover the validated update→store→sync loop and read-only UX commands:

- `tspack outdated --json`
- `tspack update --dry-run`
- `tspack update <declared-dep> --dry-run --json`
- `tspack update`
- `tspack sync`
- `tspack check --json`
- `tspack why <declared-dep>`
- `tspack why <declared-dep> --json`
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

### Claude-fooding Phase 4 native xTest smoke

The Phase 4 native xTest smoke must cover the remediated native harness model and its release-critical development loops:

- `tspack test --list`
- `tspack test --filter <listed-id>` using an ID copied from list output.
- `tspack test --compact`
- `tspack test --watch` as documented/manual or test-hooked coverage because it is long-running.
- `tspack test --batch`
- `tspack test --update-snapshots`
- `tspack test --json`
- Type assertion fixture coverage for `assert.type<TExpected>(value, reason)`.
- Snapshot fixture coverage for `expect.snapshotText(...)` and `expect.snapshotJson(...)`.

Native xTest smoke coverage should verify:

- Source TypeScript import closure participates in runtime execution.
- Test IDs are root-relative and stable between list, run, and reports.
- Copied-ID filtering selects the listed Fact or Theory case.
- Bridge override works through `--xtest-bridge` or the `--bridge` alias.
- Theory callback placement is flexible.
- Zero-case Theory diagnostics report `TSPACK_TEST_THEORY_NO_CASES` instead of silently passing.
- Compact output hides passing tests while preserving failures, skips, diagnostics, and summary counts.
- Watch mode performs dirty-key reruns without overlapping active runs.
- Snapshots update only when `--update-snapshots` is explicitly requested.
- Batch mode preserves deterministic report ordering.
- `assert.type` failures produce semantic TypeScript diagnostics.

### Claude-fooding Phase 5 RunTarget smoke

The Phase 5 RunTarget smoke must cover the remediated runtime loop for declared `RunTargets`, `tspack run`, `tspack doctor run`, and inspect-run startup reuse:

- `tspack doctor run`
- `tspack doctor run --json`
- `tspack run --list`
- `tspack run --list --json`
- `tspack run --package <pkg> <target> --once`
- `tspack run <target> --once`
- `tspack run <target> --env PORT=3001`
- HTTP readiness smoke.
- TCP readiness smoke.
- stdout-match readiness smoke.
- `cwd: "workspace"` smoke.
- `cwd: "package"` smoke.
- Status/stderr plus child stdout passthrough smoke.
- `tspack inspect --run <target> --env PORT=3001` env/cwd startup reuse smoke.
- `tspack how TSPACK_RUN_TARGET_AMBIGUOUS`.
- `tspack how TSPACK_RUN_INVALID_ENV`.

RunTarget smoke coverage should verify:

- `system` runtime is available without requiring a binary named `system`.
- Reserved `bun` and `deno` runtime backends are reported as `not_applicable` until implemented.
- Text-mode `tspack doctor run` includes useful runtime, availability, target, cwd, and readiness details.
- TSPack status/progress is written to stderr while child stdout and stderr pass through to their matching streams.
- Duplicate target names produce package-qualified ambiguity diagnostics and remediation hints.
- Effective cwd policy/path is reported for run, list, doctor, and inspect-run flows.
- Readiness details are exposed for HTTP, TCP, and stdout-match readiness policies.
- `--env` status output lists keys only and never prints values.
- `--env` values are literal after shell parsing; TSPack performs no shell interpolation.

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

## Lifecycle capability smoke

Create a fake npm registry package with a `postinstall` script that would write a marker file if executed. Run `tspack update` and verify `ts-lock.toml` records a `lifecycleScript` capability with the raw command. Run `tspack check` and verify `TSPACK_SECURITY_LIFECYCLE_SCRIPT_PRESENT` is reported as a warning. Run `tspack sync` and verify the marker file is not created.

### Lifecycle behavior harness smoke (M37b)

The release gate should keep a dedicated lifecycle behavior smoke separate from update/sync/materialization:

- valid JavaScript lifecycle fixture writes only under the package directory or probe temp directory and reports no violations;
- invalid fixtures report denied network, denied secret env read, denied child process, denied outside write, and denied outside read violations;
- unsupported shell command strings such as `sh -c ...` and `node install.js && curl ...` are rejected before execution;
- stdout, stderr, and exit code from controlled Node scripts are preserved;
- parent secret environment values are scrubbed unless tests explicitly inject sentinels;
- `tspack update`, `tspack sync`, and materialization remain non-executing even when lifecycle capabilities are detected.
