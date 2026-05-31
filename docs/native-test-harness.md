# Native TSX Test Harness (M16)

M16 introduces an opt-in, TSPack-native unit test substrate.

See `docs/claude-fooding-phase4.md` for the Phase 4 native xTest remediation closeout.

## Scope

- Native test file naming
- Native benchmark file naming: `*.benchmark.tsx`
: `*.xtest.tsx`, `*.valid.tsx`, `*.invalid.tsx`
- TSX tags are used for static discovery and metadata
- Test callback bodies are ordinary TypeScript code
- Assertions require explicit human-readable reasons
- `assert.*` is the primary assertion API
- `expect(...).matcher(...).because(reason)` is available
- Fixture, command, filesystem, golden, archive, and diagnostics helpers are intentionally out of scope in M16
- Vitest remains separate and unchanged for existing test suites

## TSX discovery model

Supported tags:

- `<Suite name="...">`
- `<Fact name="...">{() => {}}</Fact>`
- `<Theory name="..."> ... </Theory>`
- `<Case ... />`

Discovery is static and deterministic:

- names must be string literals
- case values must be literal string/number/boolean/null
- spread syntax is rejected
- dynamic test generation is not supported

## Theory structure

A valid `<Theory>` has exactly one callback body and one or more direct `<Case />` children.
The callback body may appear before the cases, after the cases, or between cases:

```tsx
<Theory name="lengths">
  {({ input, expected }) => {
    assert.equal(input.length, expected, 'length matches the case');
  }}
  <Case input="a" expected={1} />
  <Case input="abc" expected={3} />
</Theory>
```

Case suffixes are assigned from direct `<Case />` order, not callback position, so the example above lists and runs as `lengths[0]` and `lengths[1]`.
List/discovery mode remains static: it reads the TSX structure, lists valid theory cases, and does not execute the callback.

Invalid theory structures are diagnostics rather than vacuous passes:

- `TSPACK_TEST_THEORY_NO_CASES` for a callback with no direct `<Case />` children.
- `TSPACK_TEST_THEORY_MISSING_BODY` for cases with no callback body.
- `TSPACK_TEST_THEORY_DUPLICATE_BODY` for more than one callback body.
- `TSPACK_TEST_INVALID_THEORY_STRUCTURE` for unsupported direct children or non-callback expressions.

## Result IDs

Example IDs:

- `math/addition works`
- `math/string lengths[0]`
- `math/string lengths[1]`


## M17 file discovery

Native test files use the `*.xtest.tsx` suffix. Here, `x` means extended/xUnit style, not skipped.
Native tests can live next to production source files, but native test tags are only valid in native test files.
Conventional `*.test.ts`, `*.test.tsx`, `*.spec.ts`, and `*.spec.tsx` are not claimed by the native harness.
Discovery/listing is static and does not execute callback bodies.

Non-goals remain unchanged for M17: no fixture runner, no command helpers, no filesystem assertions, no Vitest orchestration, and no `tspack test` CLI command.

## M17b assertion and control-flow additions

### `assert.near(actual, expected, tolerance, reason)`

- Explicit tolerance is required; there is no implicit default.
- Reason is mandatory and must be non-empty.
- Assertion passes when `Math.abs(actual - expected) <= tolerance`.
- Assertion fails when:
  - `actual`, `expected`, or `tolerance` is non-finite.
  - `tolerance` is negative.
  - absolute difference is greater than tolerance.
- Failure uses assertion kind `near` and includes actual, expected, tolerance, absolute difference, and reason.

### `skip(reason)`

- `skip(reason)` is exported by the native test surface for runtime conditional skipping.
- Reason is mandatory and must be non-empty.
- Intended usage is guard-clause style near the top of a Fact or Theory case body.
- Calling `skip(reason)` immediately exits the current test body:
  - In `Fact`, that Fact result is `skipped`.
  - In `Theory`, only the current Case execution is `skipped`; other cases still run.
- Skips are not failures and are reported with `status: "skipped"` and a recorded skip reason.

## M17c native artifacts

`<Artifact />` is supported under `<Fact>` and `<Theory>` declarations.

```tsx
<Artifact name="report" path="report.json" format="json" />
```

Rules:
- `name` and `path` are required string literals.
- `format` is optional string literal.
- `optional={true}` marks non-required artifacts (default is required).
- Artifact paths must be relative and safe (no absolute paths, `..`, or backslashes).

Runner options now include `artifactRoot`:
- `runSuite(..., { artifactRoot })`
- `runNativeTestFiles(..., { artifactRoot })`
- Per-test output directory is `artifactRoot/<sanitized-test-id>/...`.

Test callbacks receive `ctx.artifact` writer:
- `writeText(name, text, reason)`
- `writeJson(name, value, reason)`
- `writeBytes(name, bytes, reason)`

Artifact reasons are mandatory. Required artifacts must be written unless the test is skipped.

Non-goals in M17c remain unchanged: no fixtures, no snapshots/golden assertions, no command helpers, and no filesystem assertion API beyond artifact writing.

## M17d list/filter/report

### Listing APIs

- `listDiscoveredTests(discovery)` expands static discovery into deterministic `ListedTest[]` entries.
- `listNativeTests(options)` performs file discovery and returns `{ tests, diagnostics }`.
- Listing is static-only and does not import test modules or execute callbacks.
- Listed IDs keep the `file.xtest.tsx::suite/test` prefix shape, are root-relative, normalize path separators to `/`, and are the same IDs used by run reports and filters.
- Listed metadata includes Fact and Theory cases plus declared artifacts.

### Filter semantics

- `runNativeTestFiles({ filter })` uses plain substring matching against full root-relative test IDs.
- Theory-name matches include all theory cases because case IDs include the theory name.
- Case suffix filtering (for example `[1]`) works because suffix appears in final IDs.
- A full ID copied from `tspack test --list` can be passed to `--filter` to select that test.
- Filtering is applied before module import; files with no matching discovered IDs are not imported.
- No-match filter emits `TSPACK_TEST_FILTER_NO_MATCH` and returns no test results.

### Report model

- `createNativeTestReport(result)` returns `{ summary, tests, diagnostics }`.
- Summary includes total/passed/failed/skipped/diagnostic counts.
- Test entries include status, optional skip reason, failure normalization, and artifacts.
- Failure normalization surfaces code/message/reason/assertion/actual/expected plus near details when present.

### Text, compact text, and JSON reports

- `formatNativeTestTextReport(report)` emits deterministic PASS/FAIL/SKIP lines, failure details, artifact lines, diagnostics, and summary counts.
- `formatNativeTestCompactTextReport(report)` emits compact native xTest run output: passed tests are hidden, failed tests keep full failure details, skipped tests keep their reason, diagnostics remain visible, and summary counts are always printed. It intentionally does not use dot-per-pass output, spinners, ANSI control, or terminal control sequences.
- `formatNativeTestJsonReport(report)` emits two-space indented JSON with trailing newline. Compact formatting is text-only and does not change JSON structure.
- `tspack test --watch` reuses the native xTest single-run executor from the Go CLI. The JavaScript bridge remains a single-run runner; the Go CLI handles polling, debounce, rerun orchestration, Ctrl+C/SIGTERM cancellation, and stderr watch progress messages. Watch mode supports `--filter` and `--compact`, rejects list/JSON modes, and does not implement affected-test graph selection or an interactive UI.

### Exit code semantics

- `nativeTestExitCode(report)` returns `1` when:
  - any test failed, or
  - any diagnostic is severity `error` (default severity if missing).
- Returns `0` when all tests pass/skip and diagnostics are non-error.
- `TSPACK_TEST_FILTER_NO_MATCH` is treated as error and therefore exits `1`.

M17d non-goals remain unchanged: no fixture runner, no command helpers, no filesystem assertion suite, no Vitest orchestration, and no package-manager `tspack test` CLI.


## Standalone artifacts

- `tspack artifact` runs standalone native xTest `<Artifact>` units declared directly under `<Suite>`.
- `tspack pack` creates package `.tgz` archives; it is unrelated to native test artifacts.


See also `docs/artifacts.md` for standalone artifact mode details.


## Valid/Invalid fixture invariants

- `*.valid.tsx` files allow only `<Valid>` children under `<Suite>`.
- `*.invalid.tsx` files allow only `<Invalid>` children under `<Suite>`.
- `<Valid>` and `<Invalid>` execute like facts with sync/async support and `skip(reason)`.
- Invalid invariants pass when expected diagnostics are observed (for example with `expect.error(...)`).
- Use `expect.error(subject, code).because(reason)` to assert an error code exists.
- Use `expect.noErrors(subject).because(reason)` to assert no error diagnostics exist.
- Use `expect.noError(subject).because(reason)` as singular alias of `expect.noErrors(...)`.
- Use `assert.LGTM(subject, reason)` for assert-style diagnostic cleanliness checks.
- These fixture invariants run via the native xTest backend (`tspack test -xtest`) and are not included in `tspack artifact`.

Diagnostic-clean semantics for `expect.noErrors`, `expect.noError`, and `assert.LGTM`:
- supports `Diagnostic[]`
- supports `{ diagnostics: Diagnostic[] }`
- supports `{ ok: boolean, diagnostics: Diagnostic[] }`
- `severity: "error"` fails
- missing/unknown severity fails
- `severity: "warning"` and `severity: "info"` pass
- empty diagnostics pass

Reasons are mandatory:
- expectation chains must end in `.because(nonEmptyReason)` or fail with `TSPACK_EXPECT_BECAUSE_REQUIRED`
- `assert.LGTM` requires a non-empty reason or fails with `TSPACK_ASSERT_REASON_REQUIRED`

`assert.LGTM` is intentionally narrow: it only checks whether diagnostics are error-clean; it is not a broad semantic validator.

## Project fixtures (`<Project />`)

Executable native units (`Fact`, `Theory`, `Valid`, `Invalid`, and suite `Artifact`) can include one `<Project />` declaration.

- `from?: string` fixture directory copied into per-execution sandbox.
- `name?: string` label metadata.
- `keepOnFailure?: boolean` preserves sandbox on failure.

The callback context includes `project` with `path`, `readText`, `readJson`, `writeText`, `writeJson`, and `writeBytes`. Paths are sandboxed and unsafe paths are rejected. Project write helpers require a non-empty reason.

Fixture copy skips `node_modules`, `.git`, `.tspack`, `tspack-artifacts`, and `dist-packages`. Symlinks are rejected (`TSPACK_PROJECT_SYMLINK_UNSUPPORTED`).

### Command helpers (M20)

When a unit declares `<Project />`, callback context also includes `command`:

- `command.run(args, reason, options?)`
- `command.tspack(args, reason, options?)`

Rules:
- `args` must be a non-empty string array (no shell strings).
- `reason` is required (`TSPACK_COMMAND_REASON_REQUIRED`).
- Default `cwd` is `project.rootPath`.
- `options.cwd` must be relative/safe and remain inside project root (`TSPACK_COMMAND_INVALID_CWD`).
- `options.timeoutSeconds` defaults to 30 seconds and must be positive finite.
- Non-zero exits return `CommandResult` and do not throw.
- Timeout returns `timedOut: true` with `TSPACK_COMMAND_TIMEOUT`.
- Spawn failure returns `TSPACK_COMMAND_SPAWN_FAILED`.

Each command writes durable evidence files under the unit artifact directory in `commands/`:
- `<n>.stdout.txt`
- `<n>.stderr.txt`
- `<n>.command.json`

`command.json` records args/cwd/exitCode/signal/timedOut/duration/reason/diagnostic codes and intentionally omits environment variables.

`command` is only available for Project-backed `Fact`, `Theory`, `Valid`, `Invalid`, and standalone `Artifact` units. It is not available in `Benchmark` or `Prophecy`.

`assert.exitCode(result, expected, reason)` asserts expected process exit status and fails with `TSPACK_ASSERT_EXIT_CODE_FAILED` on timeout or mismatch.

Command execution itself does not satisfy meaningful-action enforcement; tests still need assertions/expectations/skips.

### Non-goals

- No shell-string command execution helper.
- No filesystem assertion helpers.
- No snapshot/golden tooling.
- No automatic valid/invalid parser runners.

## M19e execution contracts

### `CycleTime`

Use `<CycleTime seconds={numberLiteral} />` to set execution timeout metadata.

Allowed locations:
- Under `<Theory>` (applies per theory case).
- Under suite-level standalone `<Artifact>` (applies to artifact generation unit).

Not allowed:
- Under `<Fact>`, `<Valid>`, `<Invalid>`, `<Case>`, or nested test artifact declarations.

Rules:
- `seconds` is required.
- Must be a positive finite number literal.
- No dynamic expressions.
- No JSX spread attributes.
- Only one `CycleTime` per parent.

Diagnostics:
- `TSPACK_TEST_INVALID_CYCLETIME`
- `TSPACK_TEST_DUPLICATE_CYCLETIME`
- `TSPACK_TEST_CYCLETIME_NOT_ALLOWED`

### Timeout behavior

- Default timeout is **30 seconds** for executable native units.
- Theory `CycleTime` overrides timeout per case.
- Standalone Artifact `CycleTime` overrides timeout for artifact execution.
- Custom Fact/Valid/Invalid timeout controls are intentionally out of scope for M19e.

Limitation:
- Current timeout guards async/awaited operations in the current runner.
- It does not interrupt a hard synchronous infinite loop without future worker/process isolation.

### Meaningful-action requirement

Executable units must perform meaningful action.

For Fact / Theory case / Valid / Invalid, meaningful action is at least one of:
- `assert.*`
- `expect(...).because(...)`
- `skip(reason)`

For standalone Artifact, meaningful action is at least one of:
- `assert.*`
- `expect(...).because(...)`
- `skip(reason)`
- `artifact.writeText`, `artifact.writeJson`, `artifact.writeBytes`

If a unit otherwise completes successfully without meaningful action, it fails with `TSPACK_TEST_NO_ASSERTION`.

### M19e non-goals

- benchmarks
- death tests
- worker/process isolation
- custom Fact timeout controls


## Doom (Prophecy) tests

- File naming: `*.prophecy.tsx`
- Top-level unit: `<Prophecy name="...">` with required `<Foretell reason="..." />`.
- Run with `tspack doom`; this mode is isolated from `tspack test`, `tspack bench`, and `tspack artifact`.
- Each prophecy runs in a subprocess and writes envelope artifacts (`envelope.json`, `stdout.txt`, `stderr.txt`) under doom artifact root.
- Timeout via `<CycleTime seconds={...} />` is supported; timeout is a failure (`TSPACK_DOOM_TIMEOUT`) in M19h.
- `assert.doom(...)` can validate doom results in harness tests.

See also `docs/doom.md` for detailed Prophecy/Doom semantics and future-work limits.

## M29 local import materialization

Native runtime execution now materializes a local relative module closure into temporary `.mjs` files before importing the entry module.
This is intentionally **not a bundler**.

Supported during runtime execution (`*.xtest.tsx`, `*.valid.tsx`, `*.invalid.tsx`, `*.benchmark.tsx`, `*.prophecy.tsx`, standalone `Artifact`):
- static relative `import` / `export ... from` specifiers
- `.ts`, `.tsx`, `.js`, `.jsx`
- extensionless resolution (`.ts`, `.tsx`, `.js`, `.jsx`, and `index.*` variants)
- recursive local closure loading with cycle-safe seen-set
- bare package imports are preserved and left to Node runtime resolution

Unsupported:
- path aliases / tsconfig paths
- CSS or asset imports
- dynamic non-literal imports
- package import rewrites

Safety: local relative imports must resolve inside run `rootDir`; outside-root resolution is rejected.

Discovery/list mode remains static and non-executing.

## Snapshot and golden assertions

Native xTest supports deterministic source-controlled golden files through expectation helpers:

```tsx
expect.snapshotText(value, "button-primary-class").because("button primary class output should remain stable");
expect.snapshotJson(value, "button-class-map").because("button class map should remain stable");
```

The `.because(reason)` call is mandatory and uses the same expectation-chain enforcement as other `expect` helpers. A test case whose only meaningful action is a snapshot assertion counts as having expectation activity, so it does not fail with `TSPACK_TEST_NO_ASSERTION` when the snapshot assertion executes.

Snapshots are anchored to the source test file, not to a `<Project>` sandbox. For `src/button.xtest.tsx`, snapshots live under `src/__snapshots__/button.xtest.tsx/` with filenames such as `button-primary-class.snap.txt` and `button-class-map.snap.json`. Snapshot names must be explicit safe names using letters, numbers, `_`, `-`, and `.`, must not be empty, must not start with `.`, must not contain `..`, and must not contain path separators. Invalid names fail with `TSPACK_SNAPSHOT_INVALID_NAME`; names are not sanitized.

Default `tspack test` is read-only for snapshots. A missing snapshot fails with `TSPACK_SNAPSHOT_MISSING`, and a changed snapshot fails with `TSPACK_SNAPSHOT_MISMATCH`. Run `tspack test --update-snapshots` to intentionally write missing snapshots or replace mismatched snapshots for the tests that actually run. `--filter` limits which snapshot assertions execute and therefore which files can be updated. List/discovery mode remains static and does not read, write, or execute snapshot assertions.

Text snapshots require a string value. Line endings are normalized to `\n`, and stored/compared content is guaranteed to end with a trailing newline without stripping intentional extra blank lines. Non-string values fail with `TSPACK_SNAPSHOT_TEXT_VALUE_INVALID`.

JSON snapshots use deterministic two-space serialization with sorted object keys, preserved array order, and a trailing newline. Supported values are `null`, booleans, finite numbers, strings, arrays, and plain objects. Unsupported values fail with `TSPACK_SNAPSHOT_JSON_UNSUPPORTED`, including `undefined`, functions, symbols, bigint values, `NaN`/`Infinity`, circular references, and non-plain objects.

Mismatch diagnostics include the snapshot path, reason, expected and actual SHA-256 hashes, and the first differing line with expected and actual line text. Full snapshot contents are not dumped. Compact output still hides passing tests and shows snapshot failures because they are normal failed tests. Snapshot update mode reports update activity and includes a `snapshots updated` summary count.

M34e intentionally does not add TSX `<Snapshot>` elements, inline snapshots, binary snapshots, custom serializers, interactive update prompts, snapshot pruning, Vitest snapshot compatibility, or watch-mode behavior changes.

## File-level batch execution

Native xTest supports file-level batch execution through `tspack test --batch`. Batch mode keeps the existing native harness semantics inside each file: test declarations execute sequentially, Theory cases are not parallelized, and snapshot assertions, artifacts, project fixtures, and command helpers use the same per-test contexts as serial runs.

Across files, the runner schedules up to an automatically selected worker count. The count is the smaller of available host parallelism and the number of runnable files, with an internal cap of eight workers to avoid oversubscribing small projects or CI runners. This is intentionally not a public tuning surface.

Batch output is deterministic by construction. Discovery order remains the sorted root-relative native file order. Each worker stores its file result at the file's original discovery index, then the final text, compact text, or JSON report is rendered once after all workers complete. A faster later file can finish before an earlier slower file, but the report still lists the earlier file first.

Filters are applied before scheduling wherever discovery can prove that a file has no matching tests, which avoids importing unselected files and their module side effects. Snapshot update mode remains safe because snapshot paths are namespaced by source file, and tests within a source file still execute sequentially. Project fixtures and artifact directories remain per-test/per-file deterministic and are not shared across concurrently running files.

## M34g experimental static type assertions

Native xTest now includes an experimental static assertion lane for TypeScript assignability checks:

```tsx
<Fact name="cx returns string">
  {() => {
    assert.type<string>(
      cx("a"),
      "cx should return a string"
    );
  }}
</Fact>
```

`assert.type<TExpected>(value, reason)` is a static proposition, not a runtime value comparison. The TypeScript signature is intentionally simple: `value` must be assignable to `TExpected`, and `reason` is a required human-readable string. At runtime `assert.type` validates the reason and records assertion activity, but it does not inspect or compare TypeScript types.

During native test execution, xTest builds a TypeScript `Program` from the original native test source file plus TypeScript's normal local relative import resolution. It uses controlled compiler options (`strict`, ES2022, ESNext modules, Bundler module resolution, JSX preserve, no emit) rather than a full bundler, Vite, or a custom TypeScript symbol graph engine. This is enough for local source return types imported with relative paths to participate in assignability checks.

Type assertion calls shaped like `assert.type<TExpected>(value, "reason")` are discovered in Fact/Theory callback bodies. Passing calls then execute as runtime no-ops that count as meaningful assertion activity. Failing calls are reported as native test failures with `TSPACK_TYPE_ASSERTION_FAILED`, the assertion reason, expected type text, TypeScript diagnostic code/message, file, line, and column when available. Missing or empty literal reasons are reported as `TSPACK_TYPE_ASSERTION_REASON_REQUIRED`.

List mode remains non-executing and does not run the typecheck lane. Normal run, batch run, compact output, and JSON output include selected type assertion failures. Filtered runs ignore type assertion failures in unselected Fact/Theory bodies when the assertion can be mapped to a test context; Theory diagnostics attach to the Theory base rather than to every case.

Current M34g limitations:

- Assignability only; no exact type equality helper.
- No negative type assertions such as “not assignable”.
- No `expect.type` chain API.
- No tsconfig/path-alias integration.
- No Vite integration.
- No full custom TypeScript symbol graph analyzer.
- Type assertions outside a discoverable Fact/Theory callback may only map to file-level typecheck context.

## Lifecycle behavior probe helper (M37b)

Native xTest exposes an explicit lifecycle behavior helper for JavaScript/Node lifecycle scripts:

```ts
import { lifecycle, Suite, Fact, Project, assert } from "@tspack/manifest-frontend/native-test";

export default (
  <Suite name="lifecycle">
    <Fact name="postinstall stays inside policy">
      <Project from="./fixtures/package" />
      {async ({ project }) => {
        const result = await lifecycle.runScript({
          packageDir: project.path("package"),
          command: "node install.js",
          policy: {
            denyNetwork: true,
            denyChildProcess: true,
            denyEnv: ["NPM_TOKEN"],
            allowRead: ["package/**", "tmp/**"],
            allowWrite: ["package/**", "tmp/**"],
          },
          env: {
            NPM_TOKEN: "sentinel-token",
          },
        });

        assert.equal(result.exitCode, 0, "script exit code is preserved");
        assert.equal(result.violations, [], "script stayed inside policy");
      }}
    </Fact>
  </Suite>
);
```

`lifecycle.runScript` returns:

- `exitCode`, `signal`, and `timedOut`;
- captured `stdout` and `stderr`;
- `violations`, each with a stable `code`, `kind`, `detail`, and optional `path`, `module`, or `envKey`;
- observed `reads` and `writes` where the MVP guard can record them.

Supported command shape is intentionally narrow: `node install.js` and `node ./install.js`, with optional argv after the script. The helper does not execute shell strings, `npm run`, `sh -c`, command chaining, or package-manager install compatibility behavior.

The helper runs with `cwd = packageDir`, a temporary `HOME`, temporary `TMPDIR` / `TEMP` / `TMP`, a scrubbed inherited environment, and then the explicit `env` overlay supplied by the test. Parent secrets are not inherited unless a test intentionally injects sentinel values.

Default policy denies network, child processes, common secret environment reads, and filesystem reads/writes outside `package/**` and `tmp/**`. The default denied env list includes `NPM_TOKEN`, `NODE_AUTH_TOKEN`, `GITHUB_TOKEN`, `GITHUB_ACTIONS`, AWS session keys, `VAULT_TOKEN`, `SSH_AUTH_SOCK`, `GOOGLE_APPLICATION_CREDENTIALS`, and `AZURE_CLIENT_SECRET`.

Security limitation: this is a behavior test/probe harness based on Node preload instrumentation. In the Phase 7 closeout (`docs/claude-fooding-phase7.md`), `lifecycle.runScript` is evidence/probe tooling only, not package-manager lifecycle execution or execution permission. It is not a kernel sandbox and must not be treated as safe arbitrary malware execution. Normal `update`, `sync`, and materialization paths still do not execute lifecycle scripts.

## Inspect helpers

Native xTest exposes an `inspect` helper namespace for tests that need browser-computed UI structure instead of source-level guesses. The helper reuses the same inspect backend as `tspack inspect`; it does not shell out to the CLI, take screenshots, run OCR, or mutate source. The current closeout state is summarized in [Runtime-Grounded IDE / Inspect Closeout](claude-fooding-runtime-grounded-ide.md).

```tsx
export default (
  <Suite name="UI inspect">
    <Fact name="home page exposes main landmark">
      {async () => {
        const ui = await inspect.url("http://127.0.0.1:5173", {
          browser: "chromium",
          selector: "main",
          viewport: "1280x800",
          points: [{ x: 100, y: 200 }],
        });

        assert.equal(
          ui.root?.role,
          "main",
          "main selector should resolve to a main landmark",
        );

        expect.snapshotJson(ui.root, "home-main-landmark").because(
          "main landmark structure should remain stable",
        );
      }}
    </Fact>
  </Suite>
);
```

### `inspect.url(url, options?)`

`inspect.url` launches the requested Playwright-backed browser and returns the structured inspect JSON shape used by the CLI:

- `target`
- `browser`
- `viewport`
- `root`
- `hitTests`
- `diagnostics`

Nodes may also include optional `source` metadata when the inspected page renders TSPack source hint attributes. This is useful for deterministic fixture snapshots and assertions without requiring a framework adapter:

```json
{
  "tag": "button",
  "role": "button",
  "name": "Save",
  "source": {
    "raw": "src/components/Button.tsx:42:7",
    "file": "src/components/Button.tsx",
    "line": 42,
    "column": 7,
    "component": "Button"
  }
}
```

The helper treats source metadata as inspect data only. It does not open files, trust paths, or mutate source. See [Inspect Source Mapping Design](inspect-source-mapping.md).

Supported options mirror the CLI where practical:

```ts
type InspectUrlOptions = {
  browser?: "chromium" | "webkit" | "playwright-chromium" | "playwright-webkit";
  selector?: string;
  viewport?: string | { width: number; height: number };
  points?: Array<{ x: number; y: number }>;
};
```

Browser executables are runtime dependencies. Tests that require a real browser should use the existing skip convention when Playwright cannot launch the requested browser in the current environment.

### `inspect.cdp(endpoint, options?)`

`inspect.cdp` connects to an existing Chrome DevTools Protocol endpoint, selects a target, and evaluates the shared inspect analyzer in that target without requiring VS Code or Electron in the test process.

```tsx
const ui = await inspect.cdp("http://127.0.0.1:9229", {
  target: 0,
  selector: ".statusbar",
});

assert.equal(
  ui.root?.visible,
  true,
  "VS Code status bar should be visible",
);
```

Supported CDP options are:

```ts
type InspectCdpOptions = {
  target?: number | string;
  targetUrl?: string;
  selector?: string;
  viewport?: string | { width: number; height: number };
  points?: Array<{ x: number; y: number }>;
};
```

`inspect.target` and `inspect.cdpTarget` are aliases for `inspect.cdp`.

### Observation is not an assertion

Calling `inspect.url` or `inspect.cdp` is an observation only. It does not count as a meaningful xTest action. A fact that only calls inspect still fails with `TSPACK_TEST_NO_ASSERTION`; use `assert`, `expect`, or `expect.snapshotJson` to make a claim about the returned structure.

### Inspect assertion helpers

`assert.inspect.*` provides lightweight assertions over already-collected inspect JSON. These helpers are assertions, not browser automation: they do not click, type, navigate, wait, retry, re-inspect, take screenshots, run OCR, mutate source, or validate files on disk. Like other native xTest assertions, every helper requires a non-empty reason string and counts as meaningful assertion activity.

Available helpers:

```ts
assert.inspect.exists(node, reason);
assert.inspect.visible(node, reason);
assert.inspect.hidden(node, reason);
assert.inspect.role(node, role, reason);
assert.inspect.name(node, name, reason);
assert.inspect.boundsWithin(node, constraints, reason);
assert.inspect.hitIncludes(hitTest, expected, reason);
assert.inspect.source(node, expected, reason);
```

`boundsWithin` accepts explicit min/max constraints for browser-computed `x`, `y`, `width`, and `height`:

```ts
type InspectBoundsConstraints = {
  minWidth?: number;
  minHeight?: number;
  maxWidth?: number;
  maxHeight?: number;
  minX?: number;
  minY?: number;
  maxX?: number;
  maxY?: number;
};
```

A typical fact can observe the page, find a node in the returned JSON, and assert only the facts that matter:

```tsx
const ui = await inspect.url("http://127.0.0.1:5173", {
  selector: "main",
});

const save = inspect.findByRole(ui.root, "button", "Save");

assert.inspect.visible(save, "Save button should be visible");
assert.inspect.role(save, "button", "Save control should expose button role");
assert.inspect.boundsWithin(
  save,
  { minWidth: 80, minHeight: 32 },
  "Save button should have a usable click target",
);
```

Source-hint assertions compare only the fields supplied by the test. They do not validate that a file exists:

```tsx
assert.inspect.source(
  save,
  {
    file: "src/components/Button.tsx",
    component: "Button",
    symbol: "Button.Primary",
  },
  "Save button should retain source hints",
);
```

Hit-test assertions match at least one element in a collected `hitTest.elements` list against all supplied fields:

```tsx
const ui = await inspect.url("http://127.0.0.1:5173", {
  points: [{ x: 120, y: 48 }],
});

assert.inspect.hitIncludes(
  ui.hitTests[0],
  { role: "button", name: "Save", tag: "button" },
  "click point should hit the Save button",
);
```

Failure diagnostics use inspect-specific assertion codes and include compact node or hit-test summaries rather than dumping a full subtree. Null or missing nodes fail cleanly, so tests may call `assert.inspect.visible(inspect.findByRole(...), reason)` without a separate existence assertion.

### Snapshot guidance

Inspect results are plain JSON and work with `expect.snapshotJson`. Prefer snapshotting a stable selector or subtree such as `ui.root` rather than an entire page when layout, generated IDs, or dynamic content may vary.

### Pure traversal helpers

The namespace also includes deterministic traversal helpers for plain inspect nodes:

```ts
inspect.flatten(ui.root);
inspect.findByRole(ui.root, "button", "Save");
inspect.findByText(ui.root, /Saved/);
```

## Bridge build prerequisite

Native xTest execution uses the JavaScript bridge `manifest-frontend/dist/native-test-cli.js` in the current build layout. Build it with:

```sh
cd manifest-frontend && npm run build
```

The Go CLI bridge resolver prefers `manifest-frontend/dist/native-test-cli.js` and accepts the legacy `manifest-frontend/dist/src/native-test-cli.js` path for older local checkouts. A full failing `tsc -p tsconfig.json` compile is not part of the supported native xTest smoke path.
