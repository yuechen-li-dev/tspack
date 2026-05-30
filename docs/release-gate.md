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
- `tspack inspect <url> --browser playwright-chromium --selector <css> --point x,y` selector/point regression smoke (**experimental**)
- `tspack inspect --cdp http://127.0.0.1:<port> --list-targets` with an existing CDP/Electron target smoke when available (**experimental**)
- `tspack inspect <url> --browser playwright-webkit` WebKit backend smoke, skipped when Playwright WebKit is unavailable (**experimental**)
- `tspack format`
- `tspack format --check`
- `tspack lint`
- `tspack lint --fix`
- `tspack lint --fix --unsafe`
- `tspack doctor`
- `tspack doctor security`
- `tspack doctor security --json`

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

### Claude-fooding Phase 8 format/lint smoke

The Phase 8 format/lint smoke must cover the closeout state documented in `docs/claude-fooding-phase8.md`:

- `tspack format --check`
- `tspack format`
- `tspack lint`
- `tspack lint --fix`
- `tspack lint --fix --unsafe`
- `tspack check --format`
- `tspack check --format --json`
- `tspack doctor format` format/lint backend and config reporting.
- `tspack how TSPACK_FORMAT_CHECK_FAILED`.
- `tspack how TSPACK_LINT_FIX_INCOMPLETE`.

Backend and config smoke should include:

- backend resolution through `node_modules/.bin/biome`;
- backend resolution through `node_modules/@biomejs/biome/bin/biome`;
- backend resolution through `biome` on `PATH`;
- no `biome.json` or `biome.jsonc` emits the temporary default-config stderr message;
- project `biome.json` suppresses the temporary default-config stderr message;
- project `biome.jsonc` suppresses the temporary default-config stderr message;
- executable-bit and root `.bin` materialization regression coverage.

Phase 8 expected behavior coverage should verify:

- `format --check` does not pass Biome `--check`;
- `lint` is read-only unless `--fix` is present;
- unsafe fixes require `--fix`;
- format rejects `--unsafe`;
- `check --format` does not write files;
- `check --format --json` keeps stdout clean, parseable JSON;
- project config suppresses the temporary default-config signal;
- format failures use `TSPACK_FORMAT_CHECK_FAILED` or `TSPACK_FORMAT_WRITE_FAILED` as appropriate;
- lint failures use `TSPACK_LINT_CHECK_FAILED` or `TSPACK_LINT_FIX_INCOMPLETE` as appropriate;
- invalid unsafe flag behavior is covered.

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
- `format` and `lint`: are Biome-backed lifecycle UX commands; see `docs/format-lint.md` for file-writing behavior, diagnostics, default-config signaling, and backend resolution order (`node_modules/.bin/biome`, `node_modules/@biomejs/biome/bin/biome`, then `PATH`). `tspack format --check` is the standalone read-only CI formatting gate, and `tspack check --format` adds the same read-only format validation to the main check path; both report `TSPACK_FORMAT_CHECK_FAILED` when files would change. `tspack lint` reports `TSPACK_LINT_CHECK_FAILED` for read-only lint violations, and `tspack lint --fix` reports `TSPACK_LINT_FIX_INCOMPLETE` if safe fixes may have been applied but violations remain. `tspack lint --fix --unsafe` should pass Biome `--write --unsafe` and still report `TSPACK_LINT_FIX_INCOMPLETE` on remaining violations while noting unsafe fixes were enabled. Smoke coverage should include `tspack check --format` and `tspack check --format --json` with JSON-only stdout. Smoke coverage should verify `tspack lint --unsafe` and format `--unsafe` invocations are invalid, and the default config message appears on stderr only when no project `biome.json`/`biome.jsonc` exists and stays silent when project config exists.
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

### Lifecycle acknowledgment smoke

- Create or use a package with a `postinstall` lifecycle script and verify unacknowledged `tspack check` reports `TSPACK_SECURITY_LIFECYCLE_SCRIPT_PRESENT`.
- Add a matching `<Security acknowledgedCapabilities={[...]}/>` row and verify `tspack check` no longer reports the default lifecycle warning for that exact package/script/command.
- Change the lockfile command without changing the manifest acknowledgment and verify `TSPACK_SECURITY_LIFECYCLE_SCRIPT_PRESENT` and `TSPACK_SECURITY_ACKNOWLEDGED_CAPABILITY_STALE` are reported.
- Add `behaviorFixture` and `behaviorReport` to a matching acknowledgment, verify present evidence does not warn, and verify a missing fixture reports `TSPACK_SECURITY_BEHAVIOR_FIXTURE_MISSING`.
- Verify `tspack why`, `tspack why --json`, and `tspack why --reverse` show behavior evidence metadata for the acknowledged capability.
- Verify `tspack update`, `tspack sync`, materialization, `tspack check`, `tspack doctor security`, and `tspack why` still do not execute the acknowledged script or create marker files.

### Phase 7 doctor security smoke

The lifecycle security doctor smoke must cover read-only reporting only:

- A lockfile with no lifecycle capabilities reports an `ok` lifecycle summary, zero counts in text and JSON, and exits `0`.
- An unacknowledged lifecycle capability reports a warning row with package, script, command, `execution: blocked`, and pulled-by paths when fixture edges are present; warning-only output exits `0`.
- An exact acknowledged lifecycle capability reports `ok`, `acknowledged: true`, and the acknowledgment reason without emitting an unacknowledged warning for that capability.
- A stale acknowledgment reports command drift with both acknowledged and actual commands, and an unused acknowledgment reports a separate warning.
- A missing lockfile reports a warning that package capabilities cannot be audited, recommends `tspack update`, suppresses unused-acknowledgment warnings, and keeps JSON parseable.
- `tspack doctor security --json` writes parseable two-space-indented JSON to stdout only, appends a trailing newline, and is deterministic for stable project paths.
- All-scope `tspack doctor` includes a concise `Security` section.
- The smoke must not execute lifecycle scripts, run lifecycle probes, mutate package-manager state, call registries, or run vulnerability scans.

### Claude-fooding Phase 7 security/policy smoke

The Phase 7 security smoke must cover the closeout state documented in `docs/claude-fooding-phase7.md`, including default non-execution:

- Fake npm package with `postinstall`: `tspack update` records a `lifecycleScript` capability with the raw script and command.
- `tspack check` emits `TSPACK_SECURITY_LIFECYCLE_SCRIPT_PRESENT` for an unacknowledged lifecycle capability.
- `tspack sync` and materialization do not execute the script marker.
- `tspack why npm:<pkg>@<version>` and `tspack why --json` show the capability.
- `tspack why --reverse <pkg>` shows the capability while explaining reverse reachability.
- `tspack doctor security` and `tspack doctor security --json` cover no capability, unacknowledged, acknowledged, stale, unused, and missing lock states.
- Exact `acknowledgedCapabilities` entries suppress only the matching default lifecycle warning.
- Command drift warns and does not silently trust the stale acknowledgment.
- `behaviorFixture` present and missing statuses are reported.
- `behaviorReport` present, missing, and invalid JSON statuses are reported.
- `lifecycle.runScript` valid fixtures report no violations.
- `lifecycle.runScript` invalid fixtures report network denied, env denied, child process denied, and fs read/write denied violations.
- Docs/how entries remain available for `TSPACK_SECURITY_LIFECYCLE_SCRIPT_PRESENT`, `TSPACK_LIFECYCLE_NETWORK_DENIED`, `TSPACK_LIFECYCLE_ENV_DENIED`, and behavior fixture/report diagnostics when present.

Expected behavior coverage:

- No lifecycle execution in `update`, `sync`, or materialization.
- No automatic probe execution in `check`, `doctor security`, or `why`.
- No package-name trust model or trust-by-popularity whitelist.
- OS jail support remains deferred; any future lifecycle execution must use a swappable backend seam and fail closed.
- Acknowledgments and behavior evidence remain metadata, not execution permission.

### M39b inspect extension and runtime-grounded IDE smoke

The M39b release gate should cover the VS Code extension proof of concept without requiring a live VS Code or Electron CDP target in automated tests:

- Fixture inspect JSON converts into extension tree nodes.
- Tree labels prefer role plus accessible name, then fall back to tag/text.
- Tree descriptions include compact `x,y,width,height` bounds and visible/focusable flags when available.
- Selected-node JSON copy serializes the exact inspect node payload.
- CLI command construction uses `tspack inspect --cdp <endpoint> --list-targets --json` for target discovery.
- CLI command construction uses `tspack inspect --cdp <endpoint> --target <index> --json` for target inspection.
- Missing `tspack` binary, unavailable CDP endpoint, no targets, diagnostics, and invalid JSON map to user-facing extension messages or output-channel debug details.
- `docs/runtime-grounded-ide.md` exists and is linked from `docs/inspect.md`.
- The proof of concept remains observation-first and source-safe: no visual editing, source mutation, screenshot/OCR/machine vision, framework adapters, Storybook integration, browser extension, or Code-OSS fork behavior. Source hint display, safe read-only reveal, and deterministic LLM context bundle copy are allowed because they consume existing inspect JSON and validated workspace-contained source excerpts without provider calls.



### M40c VS Code reveal-source smoke

Before release, verify safe reveal-source behavior remains narrow and read-only:

- The extension registers `tspack.inspect.revealSource` with the title **TSPack: Reveal Source for Selected Inspect Node**.
- Tree items with parsed `source.file` use the `inspectNodeWithSource` context value and expose the reveal command in the inspect tree context menu.
- A relative source hint such as `src/components/Button.tsx:42:7` opens an existing file under the selected workspace and reveals the corresponding zero-based VS Code position.
- A source hint without line/column opens the existing file at the top.
- Malformed `source.raw` with `source.parseError`, nodes with no `source`, and no selected node produce warning messages.
- Absolute paths, URL-like schemes, parent traversal, paths outside the workspace, and symlink escapes are rejected before opening.
- Missing files warn with the hinted path and are not created.
- Zero-workspace reveal warns that a workspace folder is required; multi-root reveal asks the user to choose the workspace root.
- Reveal remains read-only: no file creation, no file mutation, no visual editing, no source-map lookup, no framework adapter, and no use of `component` or `symbol` for path resolution.

## Inspect helper smoke

Before release, verify the native xTest inspect helper path:

- Run a tiny local HTML fixture with `<main role="main"><button>Save</button></main>`.
- Call `inspect.url(fixtureUrl, { selector: "main" })` and assert the returned root role/visibility.
- Snapshot a selected subtree with `expect.snapshotJson(ui.root, "...")` rather than the full dynamic page.
- Confirm `assert.inspect.role`, `assert.inspect.visible`, `assert.inspect.boundsWithin`, `assert.inspect.source`, and `assert.inspect.hitIncludes` pass against the fixture's inspect JSON.
- Confirm a fact that only calls `await inspect.url(...)` fails with `TSPACK_TEST_NO_ASSERTION`.
- Confirm a fact whose only assertion is `assert.inspect.visible(...)` satisfies no-assertion enforcement.
- Confirm inspect assertion failures include the diagnostic code, reason, compact expected/actual facts, and useful details in compact text output and JSON reports when reporting is enabled.
- Confirm browser-unavailable environments skip browser integration tests with a clear reason instead of failing CI.
- Confirm `inspect.cdp(endpoint, { target: 0, selector: "..." })` option mapping without requiring VS Code or Electron.

## M40b inspect source mapping design/probe smoke

Before release, verify the source mapping probe remains narrow and deterministic:

- `docs/inspect-source-mapping.md` exists and documents the staged strategy, non-goals, source hint contract, trust/security model, heuristic notes, and future milestones.
- `docs/inspect.md` and `docs/runtime-grounded-ide.md` link to the source mapping design.
- A static HTML fixture with `data-tspack-source`, `data-tspack-component`, and `data-tspack-symbol` reports `node.source` in inspect JSON.
- `<file>`, `<file>:<line>`, and `<file>:<line>:<column>` source hint forms parse into stable JSON fields.
- Malformed source hints preserve `source.raw` and report `source.parseError` without failing inspect.
- Nodes without source hints omit `source`.
- Extension tree conversion displays source hint metadata but must not mutate source or trust page-provided paths as authority.

### M40d LLM context bundle smoke

Before release, verify the runtime-grounded LLM context bundle remains a design/prototype milestone rather than model integration:

- `docs/llm-context-bundle.md` exists and documents the thesis, non-goals, inputs, JSON shape, trust model, size budget, author/reviewer use, and future ladder.
- The VS Code extension has a pure `buildUiContextBundle(inspectResult, selectedNode, options)` builder with deterministic serialization and no timestamps.
- Fixture inspect JSON produces a bundle with version `1`, kind `tspack.uiContext`, the exact selected node, compact ancestors, capped siblings, capped children, runtime URL/browser/viewport, constraints, and caller-supplied or inspect-result diagnostics.
- Valid workspace-contained source hints include a bounded excerpt with one-based `startLine` and `endLine`.
- Source hints with no line include only the chosen first-lines window.
- Missing files, absolute paths, parent traversal, URL-like schemes, paths outside the workspace, and symlink escapes produce validation errors and no excerpt.
- Compact names/text are truncated deterministically.
- The extension registers `tspack.inspect.copyLlmContext` with the title **TSPack: Copy Selected Inspect Node LLM Context** when the copy command is enabled.
- The copy command copies parseable JSON, uses workspace validation for source excerpts, and does not call a model, mutate source, open network connections, run `tspack check`, or perform prompt orchestration.
- Existing copy-node JSON, reveal-source, and inspect tree conversion tests continue to pass.

### M40f runtime-grounded IDE / inspect closeout smoke

The M40f release gate should cover the closeout state documented in `docs/claude-fooding-runtime-grounded-ide.md`:

- `docs/claude-fooding-runtime-grounded-ide.md` exists and summarizes the original motivation, M39a through M40e remediation, current inspect model, current VS Code extension model, current xTest inspect model, LLM context model, golden workflow, non-goals, and future ladder.
- Inspect CLI smoke verifies selector and point arguments reach the analyzer.
- CDP target discovery smoke verifies fallback through `Target.getTargets` when the HTTP target list is empty but browser-level CDP targets exist.
- WebKit smoke verifies the `playwright-webkit` or `webkit` backend alias is accepted and that browser-unavailable environments skip cleanly.
- VS Code extension smoke covers inspect JSON fixture to tree conversion, CDP target command construction, selected-node JSON copy, safe reveal-source for workspace-contained paths, unsafe path rejection, and **TSPack: Copy Selected Inspect Node LLM Context** clipboard behavior.
- Source-hint smoke covers `data-tspack-source` file/line/column parsing, malformed hints preserving parse errors without failing inspect, and no filesystem trust in the CLI analyzer.
- xTest smoke covers `inspect.url` option mapping, `inspect.cdp` option mapping, inspect-only `TSPACK_TEST_NO_ASSERTION` behavior, inspect plus assertion success, `assert.inspect.visible`, `assert.inspect.role`, `assert.inspect.boundsWithin`, `assert.inspect.source`, `assert.inspect.hitIncludes`, and `expect.snapshotJson` over a selected subtree.
- Documentation smoke verifies the runtime-grounded IDE closeout, source mapping design, and LLM context bundle design documents all exist.

Expected behavior coverage:

- No screenshots, OCR, or machine vision.
- No source mutation.
- No visual editing.
- No LLM or network call.
- No framework adapter dependency.
- Browser-backed tests skip cleanly when browsers are unavailable.
