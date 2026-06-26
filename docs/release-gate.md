# Release gate

## Smoke command checklist

### Core command surface

- `tspack init --kind <library|app> --name <package-name>`
- `tspack migrate`
- `tspack migrate --write`
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


### Migrate smoke

The migrate release smoke should cover the package.json-to-draft onboarding path, report-only evidence model, `--check` structural validation loop, and safety posture documented in `docs/migrate.md` and `docs/claude-fooding-migration.md`.

#### Output and collision behavior

- `tspack migrate --root <fixture>` is a dry-run preview, prints planned paths and summary information, and writes nothing.
- `tspack migrate --root <fixture> --write` writes `manifest.migrated.tsx` and `tspack-migration.md`.
- Existing migration outputs fail without `--force` and preserve all-or-nothing output behavior.
- `tspack migrate --root <fixture> --write --force` overwrites the migration output files only.
- `manifest.tsx`, `package.json`, lockfiles, source files, and dependency installations are not mutated.

#### Package mapping and TODO taxonomy

- The generated manifest maps package metadata, dependencies, peer dependencies, optional peer metadata, optional dependencies, dev tools, targets, and publish includes.
- Scoped and dashed packages whose TypeScript-safe dependency identifier differs from the npm package identity emit explicit keys, including `@biomejs/biome` -> `biomejsBiome` with `key: "@biomejs/biome"`, `@types/react` -> `typesReact` with `key: "@types/react"`, and `react-dom` -> `reactDom` with `key: "react-dom"`.
- Generated target `deps` and `peers` use string package identity refs consistently, while generated `<Tools>` values use dependency objects.
- Generated target `runtime` and `types` paths strip leading `./` from package file paths without rewriting `../` paths.
- Phase 8a carryover smoke runs the scoped fixture through both `tspack migrate --check --root <fixture>` and `tspack migrate --write --check --root <fixture>` and verifies no `TSPACK_IR_UNKNOWN_DEPENDENCY_REF` or `TSPACK_MIGRATE_GENERATED_IR_INVALID` diagnostics.
- The generated manifest and report include the stable TODO taxonomy, including `MIGRATION_TODO_TARGETS`, `MIGRATION_TODO_DEP_CLASSIFICATION`, `MIGRATION_TODO_RUN_TARGETS`, `MIGRATION_TODO_PUBLISH`, `MIGRATION_TODO_BOUNDARIES`, `MIGRATION_TODO_TYPES`, and `MIGRATION_TODO_SECURITY`.
- TODO-only fixtures still exit zero when validation passes; TODOs are review markers, not validation failures.

#### package-lock evidence

- A fixture with `package-lock.json` reports package-lock evidence for direct resolved versions.
- The same smoke should cover lifecycle script, bin, platform/native, peer, duplicate-version, `@types`, and approximate fanout evidence where fixtures expose those cases.
- A fixture without `package-lock.json` succeeds and reports lock evidence as not found.
- An implicit invalid lock warns and continues; explicit missing or invalid `--package-lock` fails.
- No `ts-lock.toml` is generated from package-lock evidence.

#### Source scan evidence

- Default source scan evidence reports runtime imports.
- Source scan evidence reports type-only imports.
- Source scan evidence reports imported packages missing from package.json declarations.
- Source scan evidence reports runtime imports declared only in `devDependencies` as dev-runtime mismatch evidence.
- Source scan evidence reports Node builtins separately from npm dependencies.
- `tspack migrate --root <fixture> --no-source-scan` skips source scan evidence and continues migration.

#### Scripts and RunTarget suggestions

- Script classification reports runtime target suggestions / RunTarget suggestions for likely runtime or dev-server scripts.
- Build, test, lint, format, package, maintenance, and lifecycle scripts are listed as report-only evidence and are not migrated into active RunTargets.
- Shell syntax, environment assignment, and command-shape review notes are visible in the report when applicable.
- Scripts are listed but not executed; no script execution is permitted during migrate smoke.

#### `migrate --check` validation

- `tspack migrate --check` validates a generated draft from the current root without writing migration outputs.
- `tspack migrate --root <fixture> --check` validates the generated draft, reports frontend/IR pass/fail and TODO counts, writes no files, does not execute scripts, and does not create `ts-lock.toml`. Include a scoped tool fixture with `@biomejs/biome` and an `@types/react` fixture to verify explicit keys and generated-IR validation pass.
- `tspack migrate --root <fixture> --write --check` validates before writing and writes `manifest.migrated.tsx` and `tspack-migration.md` only when validation passes.
- A validation-failure fixture exits nonzero and writes no migration outputs.
- TODOs are counted but are not failures.
- `migrate --check` does not run update, sync, package installation, package materialization, or package script execution.

#### Safety posture

- No npm install or package-manager install is run.
- No package scripts are run.
- No source files are mutated.
- No package.json files are mutated.
- No lockfiles are mutated or generated.
- No LLM calls are made.



### M52a self-hosting dogfood gate

The M52a self-hosting gate proves that TSPack can manage and check this repository after the existing source bootstrap path has produced a working CLI/frontend pair:

- A root `manifest.tsx` exists and models the Go CLI/backend, manifest frontend, VS Code extension, dogfood examples, and release/install script surfaces.
- `tspack run --list --root .`, `tspack check --root .`, `tspack check --format --root .`, `tspack doctor security --root .`, `tspack outdated --root .`, and `tspack update --policy --dry-run --root .` work against the repository contract after bootstrap.
- The bootstrap boundary is documented: existing Go/npm build paths create the first usable CLI/frontend; self-hosting does not claim a zero-bootstrap build.
- Root `Security` and `UpdatePolicy` declarations are present when feasible and remain conservative.
- Generated artifacts stay ignored, and read-only self-host commands do not unexpectedly mutate tracked repository state.
- `scripts/self-host-smoke.sh` covers the routine read-only command matrix and `scripts/self-host-smoke.sh --release` covers the optional release build.
- Existing direct validation remains intact: `npm --prefix manifest-frontend run build`, `go test ./...`, and `./scripts/build-release.sh`.

### M52b self-host closeout and release-readiness gate

The M52b closeout gate keeps the 0.1.0 self-hosting story honest, reproducible, documented, and release-gateable without adding new product features:

- `./scripts/self-host-smoke.sh` passes from the repository root.
- `./scripts/self-host-smoke.sh --release` remains available for release/manual validation and is not run accidentally by routine smoke.
- The self-host smoke has clear `--help` usage, fails clearly for missing tools or wrong working directory, builds the manifest frontend bridge, runs the read-only command matrix, and fails on tracked repository mutation.
- `docs/self-hosting.md` declares the bootstrap boundary: TSPack does not claim a zero-bootstrap build, does not remove package-manager compatibility files, does not model Go modules as npm dependencies, and does not replace release CI in 0.1.0.
- `docs/release-0.1.0.md` exists and lists concrete pre-tag commands, distribution checks, self-host checks, known deferred items, and tagging notes.
- `README.md` points to the self-hosting and 0.1.0 release-readiness docs without claiming production stability or zero-bootstrap self-hosting.
- Deferred post-0.1.0 items are documented, including inspect/browser deep testing, policy-driven mutation, targeted policy planning, setup-tspack hosted smoke after first release, and `get.tspack.dev`.
- No tracked mutation occurs during routine self-host smoke; ignored generated artifacts may still be created.
- `setup-tspack` live smoke remains deferred until the first public release exists.
- Inspect/browser deep testing remains deferred post-0.1.0.

### M45c Unix install script gate

The M45c installer gate covers `scripts/install.sh` as the first human Unix install path for GitHub Release artifacts:

- `sh -n scripts/install.sh` must pass.
- The installer must resolve the latest release from the GitHub Releases API when `TSPACK_VERSION` is unset, and must honor `TSPACK_VERSION=vX.Y.Z` or `--version vX.Y.Z` for explicit installs.
- Platform mapping must cover Linux `x86_64` / `amd64` -> `linux-amd64`, Linux `aarch64` / `arm64` -> `linux-arm64`, macOS `x86_64` / `amd64` -> `darwin-amd64`, and macOS `aarch64` / `arm64` -> `darwin-arm64`. Unsupported OS/arch combinations must fail clearly with the detected values.
- Checksum verification against the release `checksums.txt` is mandatory. Missing checksum files, missing artifact entries, checksum-tool absence, and hash mismatches must fail rather than install.
- The default install destination is `$HOME/.local/bin/tspack`; `TSPACK_INSTALL_DIR` and `--install-dir` may override it. The installer must not require `sudo` by default.
- If the install directory is not on `PATH`, the installer should print manual shell-profile guidance and must not edit shell profiles automatically.
- Windows is outside `scripts/install.sh`; the gate requires a clear note that Windows users should download `tspack-windows-amd64.zip` from GitHub Releases manually.
- The docs must not advertise `get.tspack.dev` as live. It remains a future installer endpoint TODO until hosting is implemented.
- Fake release-server tests should cover latest-version resolution, explicit-version installation, platform mapping, successful checksum verification, and checksum mismatch failure when feasible.

### M45d setup-tspack GitHub Action gate

The M45d setup action gate covers `.github/actions/setup-tspack` as the first GitHub Actions install path for GitHub Release artifacts:

- `.github/actions/setup-tspack/action.yml` must use the Node 20 runtime and point at the small handwritten installer without generated `node_modules` or dependency bundles.
- The action must document the first-party subdirectory usage path: `yuechen-li-dev/tspack/.github/actions/setup-tspack@v1`.
- `version: latest` must resolve the release tag from the GitHub Releases API, while an explicit tag such as `v0.1.0` must bypass latest resolution.
- The action must download M45b artifacts from GitHub Releases and must not build TSPack from source, use `get.tspack.dev`, install npm packages, or implement other distribution channels.
- Platform mapping tests must cover Linux `x64`/`arm64`, macOS `x64`/`arm64`, Windows `x64`, and a clear Windows `arm64` unsupported failure.
- Checksum verification against release `checksums.txt` is mandatory. Missing artifact entries and hash mismatches must fail before extraction or installation.
- Installation must extract the archive, find `tspack` or `tspack.exe`, copy it into the install directory, append the install directory to `GITHUB_PATH`, and expose `version` and `path` outputs.
- The post-install check may run `tspack --help`; it must not run project-specific commands such as `tspack check` by default.
- Normal CI should not depend on a nonexistent public release. A live workflow smoke with `version: latest` is deferred until a real release exists or is manually triggered with a known published tag.
- `node .github/actions/setup-tspack/test.js` must cover platform mapping, checksum parsing and mismatch behavior, latest tag parsing, explicit-version bypass, URL construction, extracted-binary discovery, and check-input parsing.

### M46a runtime UX gate

The M46a runtime UX gate keeps runtime profile semantics legible without changing execution behavior:

- `docs/run.md` explains RunTarget `node` / `nodejs`, `system`, `bun`, and `deno` runtime semantics, including that `node` resolves bare local tools but does not prepend `node` to path-containing script files.
- `tspack run --list` human output includes concise runtime notes for `node` and `system` behavior, explicit RunTarget runtime precedence, the workspace runtime profile, and the current "unspecified RunTarget runtime inheritance: not enabled" state.
- `tspack run --list --json` remains stable machine-readable output and does not include human prose.
- `tspack doctor runtime` text output includes `packageManagerDelegated: false` and the ownership note that TSPack owns package resolution, lockfiles, sync/materialization, check, pack, and lifecycle policy.
- Explicit RunTarget runtime precedence remains tested; workspace runtime profiles do not override explicit RunTarget runtime values.
- Runtime execution behavior is not changed: no runtime inheritance, no `tspack run --profile`, no automatic `node` prepending, no package-manager delegation, and no resolver/migration/check policy changes.

### Resolver Phase 8a carryover smoke

The resolver/update smoke should stay offline and use fake registries and fixture tarballs:

- Scoped metadata URL coverage verifies `@biomejs/biome`, `@types/react`, and `@babel/core` request `/@biomejs%2Fbiome`, `/@types%2Freact`, and `/@babel%2Fcore` respectively.
- Scoped metadata URL coverage fails on `%25`, `%2540`, or `%252F` double encoding.
- Tarball package metadata coverage accepts `package/package.json`, `babel__core/package.json`, and `estree/package.json`.
- Tarball package metadata coverage rejects or ignores deep `package/subdir/package.json` entries and fails deterministically on multiple top-level roots.
- The composed scoped URL plus non-standard tarball-root smoke resolves `@types/babel__core` from `/@types%2Fbabel__core` with `babel__core/package.json`.
- Existing unscoped package and custom registry path-prefix happy paths remain covered.

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
- npm resolver tarball metadata smoke covers package metadata under `package/package.json`, `babel__core/package.json`, and `estree/package.json`; it must reject multiple-root ambiguity and must not accept deep nested `package/subdir/package.json` as the tarball package metadata entry.
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

### M42b Node.js runtime baseline smoke

The M42b runtime portability release smoke must prove the Node.js profile is the explicit behavior-preserving baseline before any Bun/Deno execution milestone lands:

- Omitted workspace runtime and `runtime="nodejs"` normalize to the same `nodejs` IR/default.
- `tspack check` passes for omitted-runtime and explicit-`nodejs` fixtures with equivalent diagnostics.
- `tspack pack` and `tspack pack --verify` remain deterministic, and generated npm `package.json` output does not include workspace runtime profile metadata.
- `tspack why`, `tspack why --json`, and `tspack why --reverse` explain dependencies the same way under explicit `nodejs`.
- Explicit `RunTarget.runtime` and `RunTarget.cwd` values are not overridden by workspace `runtime="nodejs"`; omitted RunTarget defaults remain unchanged and no inheritance is introduced.
- Native xTest still uses the existing Node bridge path; `runtime="nodejs"`, `runtime="bun"`, and `runtime="deno"` do not switch the xTest bridge in M42b.
- `tspack format`, `tspack lint`, and `tspack check --format` keep the same Biome backend resolution and arguments under the Node.js baseline.
- Inspect helper/CLI behavior remains unchanged by workspace runtime profile where feasible to smoke.
- `tspack doctor runtime --json` reports selected `nodejs`, executable `node`, `lifecycleOwner: tspack`, and `packageManagerDelegated: false` for both omitted and explicit Node.js fixtures.
- No command delegates package-manager work to npm, Bun, or Deno because of workspace runtime profile.
- Lockfile semantics, materialization, update/sync, and pack metadata stay unchanged.
- `tspack migrate` keeps generated drafts quiet by omitting `runtime="nodejs"`; future runtime clues may be documented separately, but M42b must not infer Bun/Deno from `packageManager`.

### M42c Bun runtime proof smoke

The M42c runtime portability release smoke must prove Bun as a runtime profile without changing package-manager semantics:

- `tspack doctor runtime --json` for `<Workspace runtime="bun">` reports `selected: bun`, `executable: bun`, `status: experimental`, `lifecycleOwner: tspack`, `packageManagerDelegated: false`, `dependencyResolution: TSPack`, `lockfile: ts-lock.toml`, `materialization: TSPack`, and `securityPolicy: TSPack`.
- A PATH-controlled missing-Bun smoke reports `available: false` deterministically without warning noise for non-selected runtimes.
- A fake Bun executable smoke for `RunTarget.runtime: "bun"` proves `tspack run --once` invokes `bun` with the declared argv payload, preserves cwd, preserves `--env` overlays, passes stdout/stderr through, and satisfies stdout-match readiness.
- Missing Bun for an explicit Bun RunTarget fails before execution with `TSPACK_RUN_RUNTIME_NOT_FOUND` and does not fall back to Node.js, system execution, npm, npx, or package scripts.
- Workspace `runtime="bun"` does not override explicit `runtime: "system"` or existing Node RunTarget behavior; omitted RunTarget runtime behavior remains unchanged.
- Native xTest and JavaScript bridge paths still use the existing Node bridge seams.
- Guardrail review/tests verify no `bun install`, `bun add`, `bun pm`, `bun.lockb`, Bun dependency resolution, Bun materialization, or Bun package-manager command path was introduced.

### M42d Deno runtime proof smoke

The M42d runtime portability release smoke must prove Deno as a runtime profile without changing package-manager or task-runner semantics:

- `tspack doctor runtime --json` for `<Workspace runtime="deno">` reports `selected: deno`, `executable: deno`, `status: experimental`, `lifecycleOwner: tspack`, `packageManagerDelegated: false`, `dependencyResolution: TSPack`, `lockfile: ts-lock.toml`, `materialization: TSPack`, and `securityPolicy: TSPack`.
- A PATH-controlled missing-Deno smoke reports `available: false` deterministically without warning noise for non-selected runtimes.
- A fake Deno executable smoke for `RunTarget.runtime: "deno"` proves `tspack run --once` invokes `deno` with the declared argv payload, preserves cwd, preserves `--env` overlays, passes stdout/stderr through, and satisfies stdout-match readiness.
- Missing Deno for an explicit Deno RunTarget fails before execution with `TSPACK_RUN_RUNTIME_NOT_FOUND` and does not fall back to Node.js, Bun, system execution, npm, npx, package scripts, or `deno task`.
- Workspace `runtime="deno"` does not override explicit `runtime: "system"`, `runtime: "bun"`, or existing Node RunTarget behavior; omitted RunTarget runtime behavior remains unchanged.
- Native xTest and JavaScript bridge paths still use the existing Node bridge seams.
- Guardrail review/tests verify no `deno task`, `deno install`, `deno add`, `deno cache`, `deno vendor`, `deno.json`, `deno.lock`, import-map handling, JSR resolver, `npm:` resolver, Deno materialization, or Deno package-manager command path was introduced.
- Existing Node.js and Bun baseline smokes continue to pass.

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
- M49a targeted update fixture preserving non-selected peer-resolved multi-version lock entries, including a second React-family version such as `npm:react@19.2.7`, `npm:react-dom@19.2.7`, and `npm:scheduler@0.27.0`, while updating an unrelated selected dependency.
- M49a targeted update invariant check: removed package keys must be a subset of the selected dependency update closure, and an unrelated targeted update must not reduce the non-selected package key set.
- M49a already-current targeted update no-ops successfully without rewriting the lockfile.
- M49a targeted dry-run JSON reports `changed` as `true` or `false`, never `null`, with zero added/removed/changed counts on no-op targeted plans.
- M49a compatibility checks keep unsupported workspace/path targets rejected, target-not-found remediation intact, and full workspace update behavior unchanged.
- `tspack update <declared-dep> --root <fixture> --dry-run --json` with JSON-only stdout.
- `tspack outdated --root <fixture> --json` using metadata-only registry access.
- Scoped npm metadata URL smoke for `@types/react` and `@biomejs/biome`, verifying fake-registry requests use `/@types%2Freact` and `/@biomejs%2Fbiome` without `%25` double encoding.

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

M53c materializer graph-safety gate:

- circular dependency materialization fixture syncs without `TSPACK_MATERIALIZE_WRITE_FAILED`;
- shared dependency graph materializes bounded paths for each required strict-layout entry;
- no repeated infinite `node_modules/a/node_modules/b/...` cycle pattern appears;
- `TSPACK_MATERIALIZE_PATH_DEPTH_EXCEEDED` guard is covered for an absurd synthetic chain;
- self-host smoke still passes;
- Windows path-depth safety is considered with bounded component-depth assertions.

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
- Targeted update output includes the selected dependency context and reports no-op targeted plans without lockfile writes when the selected dependency is already wanted/current.
- JSON modes keep stdout machine-readable; progress is suppressed or kept off stdout.
- `--quiet` suppresses update progress/status lines while leaving diagnostics and errors on stderr.
- `run --once` and test harnesses that wait for stdout/stderr readiness must terminate the whole launched process tree after readiness, so inherited stdout/stderr pipes cannot keep CLI tests waiting forever.

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

### Migrate script classification smoke

The migrate release smoke must cover package.json script classification without executing scripts:

- `tspack migrate --write --root <fixture>` on a fixture containing runtime, build, test, lint, format, lifecycle, env-prefix, and shell-composite scripts.
- Verify `tspack-migration.md` contains `## Scripts and RunTarget suggestions`.
- Verify likely runtime/dev-server scripts appear in `### Suggested RunTarget candidates` with argv/readiness TODO evidence.
- Verify build/test/lint/format/package scripts appear only as non-RunTarget evidence.
- Verify lifecycle scripts appear in the security section and no marker-file script is executed.
- Verify `manifest.migrated.tsx` contains `MIGRATION_TODO_RUN_TARGETS` but no active `<RunTargets>` rows by default.

### M42 runtime profile smoke

Release gate smoke should cover the closed M42 runtime profile arc documented in `docs/claude-fooding-runtime-profiles.md`.

#### Workspace runtime parsing

- `<Workspace runtime="nodejs">`, `<Workspace runtime="bun">`, and `<Workspace runtime="deno">` parse and validate.
- Omitted workspace runtime defaults to `nodejs` in normalized IR / Go model.
- Invalid runtime profile values such as `npm`, `pnpm`, and `yarn` are rejected with `TSPACK_MANIFEST_INVALID_RUNTIME_PROFILE` or the manifest validation equivalent.

#### `doctor runtime`

- `tspack doctor runtime --json` reports selected `nodejs`, `bun`, and `deno` for matching fixtures.
- JSON shape includes `selected`, `executable`, `available`, `status`, `lifecycleOwner: "tspack"`, `packageManagerDelegated: false`, `dependencyResolution: "TSPack"`, `lockfile: "ts-lock.toml"`, `materialization: "TSPack"`, and `securityPolicy: "TSPack"`.
- Missing non-selected Bun/Deno executables do not create warning noise.

#### Node baseline

- Omitted workspace runtime and explicit `<Workspace runtime="nodejs">` normalize equivalently, including `Workspace.Runtime`.
- Existing behavior is preserved for check, pack, why, run, native xTest, inspect/JS bridge paths, and doctor.
- Runtime profile metadata does not appear in generated package metadata.

#### Bun proof

- Explicit `RunTarget.runtime: "bun"` invokes `bun <declared argv>`, preserves cwd/env/readiness behavior, and does not run package scripts.
- Missing Bun fails before fallback with `TSPACK_RUN_RUNTIME_NOT_FOUND`.
- Guardrails verify no `bun install`, `bun add`, `bun pm`, Bun lock handling, Bun resolver behavior, Bun materialization, or Bun package-manager command path was introduced.

#### Deno proof

- Explicit `RunTarget.runtime: "deno"` invokes `deno <declared argv>`, preserves cwd/env/readiness behavior, and does not run `deno task`.
- Missing Deno fails before fallback with `TSPACK_RUN_RUNTIME_NOT_FOUND`.
- Guardrails verify no `deno task`, `deno install`, `deno add`, `deno cache`, `deno vendor`, deno.json handling, deno.lock handling, import-map handling, JSR resolver, `npm:` resolver, Deno materialization, or Deno package-manager command path was introduced.

#### Runtime switch fixture

- `fixtures/valid/runtime-switch-nodejs/manifest.tsx`, `fixtures/valid/runtime-switch-bun/manifest.tsx`, and `fixtures/valid/runtime-switch-deno/manifest.tsx` differ only by the `<Workspace runtime="nodejs" | "bun" | "deno">` value.
- Normalized IR for the three fixtures differs only in `Workspace.Runtime`.
- `tspack check`, `tspack pack`, `tspack pack --verify`, `tspack why left-pad`, and `tspack why --json left-pad` stay stable across runtime profiles.
- Generated `package/package.json` metadata stays stable and does not contain workspace runtime profile metadata.
- Explicit runtime RunTargets stay explicit and stable: Node/local-bin semantics for `runtime: "node"`, direct argv execution for `runtime: "system"`, `bun <declared argv>` for `runtime: "bun"`, and `deno <declared argv>` for `runtime: "deno"`.

#### Guardrails

- No package-manager delegation is introduced; the release gate must preserve the explicit phrase: no package-manager delegation. `packageManagerDelegated` remains `false`.
- No workspace-runtime inheritance into RunTargets exists yet.
- No native xTest runtime switching or JavaScript bridge/runtime switching is introduced.
- No materializer, resolver, lockfile schema, package metadata, package-manager mutation, npm/npx fallback, Bun package-manager, or Deno task/cache/vendor behavior changes are introduced.

### M42e runtime switch demo smoke

The M42e runtime portability release smoke must prove the one-line switch demo without introducing package-manager switching:

- `fixtures/valid/runtime-switch-nodejs/manifest.tsx`, `fixtures/valid/runtime-switch-bun/manifest.tsx`, and `fixtures/valid/runtime-switch-deno/manifest.tsx` differ only by the `<Workspace runtime="nodejs" | "bun" | "deno">` value.
- Normalized IR for the three fixtures differs only in `Workspace.Runtime`.
- `tspack check` is stable across the three runtime profiles and does not produce runtime-profile noise.
- `tspack pack` and `tspack pack --verify` are stable across the three runtime profiles; generated `package/package.json` does not contain workspace runtime profile metadata such as `nodejs`, `bun`, or `deno`.
- `tspack why left-pad` and `tspack why --json left-pad` explain the same dependency graph across runtime profiles.
- `tspack doctor runtime --json` reports selected `nodejs`, `bun`, and `deno` for the corresponding fixture and keeps `lifecycleOwner: tspack` and `packageManagerDelegated: false` for all three.
- Explicit runtime RunTargets use their declared runtime: Node/local-bin semantics for `runtime: "node"`, `bun <declared argv>` for `runtime: "bun"`, `deno <declared argv>` for `runtime: "deno"`, and direct argv execution for `runtime: "system"`.
- Workspace runtime does not override explicit target runtime; for example, `runtime="deno"` at the workspace level does not change a `runtime: "bun"` RunTarget.
- Missing explicit Bun or Deno executables fail with `TSPACK_RUN_RUNTIME_NOT_FOUND`; missing non-selected runtimes do not affect check, pack, or why.
- Guardrail review/tests verify no `bun install`, `bun add`, `bun pm`, `deno task`, `deno install`, `deno add`, `deno cache`, `deno vendor`, Bun lockfile handling, Deno lockfile handling, resolver changes, materializer changes, package-manager mutation, npm/npx fallback, native xTest runtime switching, or JavaScript bridge runtime switching was introduced.
- `tspack migrate` continues to omit `runtime="nodejs"` by default and does not infer Bun or Deno runtime from package-manager or runtime-owned config files.

## Dogfooding smoke: Runtime Switch Notes

Before release, run the command matrix in `examples/runtime-switch-notes/DOGFOODING.md` or refresh it when behavior changes. The sample links runtime-profile switching, RunTargets, native xTest, inspect source hints, pack verification, why, security, and format/lint checks in one intentionally small project.

M43b update/store regression smoke must also verify:

- `go run ./cmd/tspack update --root examples/runtime-switch-notes` completes without recursively copying `.tspack/store` into itself.
- A second consecutive `go run ./cmd/tspack update --root examples/runtime-switch-notes` is idempotent and reports `lockfile diff: +0 -0`; generated `ts-lock.toml` content must not feed back into workspace/path source hashes.
- Local workspace/path source store population excludes TSPack-managed internal artifact directories such as `.tspack/` and `tspack-artifacts/`, plus generated `ts-lock.toml`, while preserving package content such as checked-in `dist/**` files.
- The local source copy helper skips a destination subtree when the copy destination is inside the source root, so store layout changes cannot reintroduce self-copy recursion.


## Inspect URL backend smoke

Release smoke must include URL inspect routing that does not depend on VS Code discovery:

- Start the `examples/runtime-switch-notes` node server and run `tspack inspect http://127.0.0.1:4171 --json`.
- Run `tspack inspect http://127.0.0.1:4171 --selector main --json` against the same server.
- Verify ordinary URL inspect reaches the Playwright URL backend; `TSPACK_INSPECT_VSCODE_NOT_FOUND` is reserved for explicit VS Code/platform-webview or related editor/backend probes.
- If Playwright browser executables are unavailable in the environment, record the browser/backend-unavailable diagnostic as an environment limitation, not as a VS Code discovery failure.

## M43d bridge build/discovery smoke

- Canonical bridge prerequisite: run `cd manifest-frontend && npm run build` from the repository root before Go CLI smoke tests that need JavaScript bridge artifacts.
- The build must emit all TSPack-required bridge entrypoints in the current layout:
  - `manifest-frontend/dist/cli.js`
  - `manifest-frontend/dist/native-test-cli.js`
  - `manifest-frontend/dist/inspect-cli.js`
- Go bridge discovery must prefer `manifest-frontend/dist/<bridge>.js` and still accept legacy `manifest-frontend/dist/src/<bridge>.js` for local/dev compatibility.
- Release smoke should include `test -f` checks for all three bridge files, `go run ./cmd/tspack test --root examples/runtime-switch-notes`, and an inspect command that proves lookup reaches the inspect backend. In environments without Playwright browser executables, `TSPACK_INSPECT_BROWSER_LAUNCH_FAILED` is acceptable; bridge-missing diagnostics are not.
- Native xTest discovery for the sample should report the root `tests/**` facts once and must not rediscover generated copies under `.tspack/store` or `tspack-artifacts`.


## M45a.1 release binary verification polish gate

- `go test ./cmd/tspack -run TestDoctorFormatReportsDefaultBiomeConfigSource -count=1 -v -timeout 180s` must pass, or any remaining timeout must be documented with a concrete unrelated blocker and reproduction.
- `go test ./cmd/tspack -count=1 -v -timeout 180s` must pass, or any remaining command-surface blocker must be classified before release.
- `./scripts/build-release.sh` must restore `manifest-frontend/dist` with trap cleanup after success, command failure, smoke failure, or handled interrupts. A pre-existing `manifest-frontend/dist.release-smoke-bak` is a hard error because the script must not overwrite or delete user data.
- The release inspect no-dist smoke must distinguish bridge lookup failures from acceptable later-stage inspect diagnostics. Missing bridge diagnostics such as `TSPACK_INSPECT_BRIDGE_MISSING`, `TSPACK_*BRIDGE*NOT_FOUND`, `manifest frontend bridge not found`, or `inspect bridge not found` fail the release smoke. Stable post-bridge diagnostics such as `TSPACK_INSPECT_BROWSER_LAUNCH_FAILED`, Playwright load failures, page-load failures, invalid-target failures, or the generic inspect failure wrapper are acceptable in constrained environments.
- Generated release assets must remain ignored and untracked: `internal/embeddedbridges/generated_assets.go`, `internal/embeddedbridges/assets/`, `manifest-frontend/dist/`, extension build output, and `dist/tspack`.

## M45a v2 self-contained release binary gate

- Release binaries must be built with `./scripts/build-release.sh`, which builds manifest frontend bridges, generates ignored embedded bridge assets, and builds with `-tags tspack_embedded_bridges`.
- Self-contained smoke must temporarily hide `manifest-frontend/dist` and run:
  - `./dist/tspack check --root examples/runtime-switch-notes`
  - `./dist/tspack test --root examples/runtime-switch-notes`
  - `./dist/tspack inspect ... --json` far enough to prove the inspect bridge resolves. Browser/backend environment diagnostics are acceptable; bridge-not-found diagnostics are not.
- Generated asset guard:
  - no committed `manifest-frontend/dist` output
  - no committed `extensions/*/dist` output
  - no committed copied bridge JavaScript blobs under `internal/embeddedbridges/assets`
  - `internal/embeddedbridges/generated_assets.go` is generated and ignored by Git

## M45b GitHub Releases artifact gate

M45b release automation must verify:

- `.github/workflows/release.yml` exists and is triggered only by `v*` tag pushes or manual `workflow_dispatch` runs.
- The release matrix builds `linux/amd64`, `linux/arm64`, `darwin/amd64`, `darwin/arm64`, and `windows/amd64` targets.
- Every target is built with `-tags tspack_embedded_bridges` after `manifest-frontend/dist` is built and `go run ./tools/generate-embedded-bridges` has produced ignored embedded bridge assets.
- Release archives are named `tspack-linux-amd64.tar.gz`, `tspack-linux-arm64.tar.gz`, `tspack-darwin-amd64.tar.gz`, `tspack-darwin-arm64.tar.gz`, and `tspack-windows-amd64.zip`.
- Archives contain the platform binary plus repository `LICENSE` and `README.md` when present; Unix tarballs preserve the executable bit.
- The `linux/amd64` workflow build extracts the archive, hides `manifest-frontend/dist`, and runs `check`, `test`, and `inspect --json` smokes far enough to prove embedded bridge resolution. Bridge-missing diagnostics are failures; stable later inspect diagnostics are acceptable.
- `checksums.txt` is generated with SHA256 entries for all uploaded archives using `<sha256>  <filename>` lines.
- GitHub Release upload is configured with `contents: write` and uploads all archives plus `checksums.txt` without deleting unrelated release assets.
- Generated embedded assets, bridge bundles, release binaries, release archives, frontend `dist`, and extension build outputs remain ignored and uncommitted.

### M46b check warning-volume gate

The M46b gate covers default human `tspack check` readability for noisy informational warning families without weakening diagnostics or security posture:

- Default human `tspack check` summarizes multiple `TSPACK_LOCK_VERSION_CONFLICT` warnings with a count, deterministic examples, and `--show-conflicts` reveal guidance.
- `tspack check --show-conflicts` reveals full individual version conflict diagnostics.
- Default human `tspack check` summarizes multiple `TSPACK_SECURITY_LIFECYCLE_SCRIPT_PRESENT` warnings with a count, deterministic examples, blocked-by-policy posture, and `--show-lifecycle` reveal guidance.
- `tspack check --show-lifecycle` reveals full individual lifecycle script diagnostics, including script and pull-chain details.
- Serious security diagnostics and all error-level diagnostics remain visible in default human output.
- `tspack check --json` remains complete and includes every individual conflict and lifecycle diagnostic.
- Warning summarization must not change exit-code semantics, diagnostic codes, lockfile schema, resolver behavior, lifecycle execution behavior, or security policy.

## M47a RunTarget runtime inheritance gate

- Unspecified RunTargets inherit the workspace runtime profile.
- Explicit RunTarget runtime wins over workspace runtime.
- No workspace runtime defaults unresolved RunTargets to `nodejs`.
- `tspack run --list` shows the resolved runtime source.
- `tspack run --list --json` includes `runtimeSource` when available.
- `tspack doctor runtime` mentions RunTarget inheritance and explicit precedence.
- Switching workspace runtime leaves pack/check/why/lock/package output unchanged.
- Node auto-prepending for JavaScript script paths is not added.
- Package-manager delegation to Bun/Deno/npm is not added.

## M48a security acknowledgment and behavior fixture correctness gate

- Split workspace parsing preserves root `security` metadata from the root manifest while merging package manifests.
- Split workspace lifecycle acknowledgments suppress or mark matching lifecycle capabilities according to current security behavior.
- `tspack doctor security` sees split-manifest acknowledgments and reports acknowledged/stale/unused counts from the merged IR.
- Native xTest exposes `runLifecycleScript` as a file global while preserving the imported `lifecycle.runScript` helper.
- A behavior fixture using the `runLifecycleScript` global passes through the native xTest backend; it remains behavior evidence only, not an OS jail, and it preserves package-manager non-execution.
- Native xTest globals are documented, and `ctx` is explicitly not documented as a global.
- `tspack how TSPACK_TEST_MODULE_LOAD_FAILED` mentions available globals and the `runLifecycleScript` behavior-fixture helper.


### M48b lifecycle script kind classification gate

- Lifecycle scripts are classified as `consumer-install`, `maintainer-publish`, or `other`.
- Default `tspack check` summaries report category counts and deterministic examples without hiding serious security diagnostics.
- `tspack check --show-lifecycle` reveals individual lifecycle category details, commands, and pull chains.
- `tspack check --json` includes individual lifecycle diagnostics with `lifecycleCategory` and `consumerInstallTime`.
- `tspack doctor security` text and JSON include lifecycle category counts and per-capability classification.
- Acknowledged, stale, unused, and behavior-fixture/report evidence behavior remains package/script/command based.
- Lifecycle execution remains blocked by default.
- No batch suppression, `acknowledgedScriptKinds`, or `--no-security-warnings` policy is implemented.

## M48c lifecycle category acknowledgment policy gate

- Manifest `Security.acknowledgedLifecycleCategories` supports `category`, optional `scripts`, and required `reason` fields.
- A `maintainer-publish` category acknowledgment suppresses default human check lifecycle noise for maintainer-side scripts only.
- Consumer-install scripts remain visible unless a consumer-install category acknowledgment explicitly matches them.
- `tspack doctor security` reports lifecycle category acknowledgments, matched category-acknowledged capability counts, and per-capability acknowledgment kind.
- Unused lifecycle category acknowledgments emit `TSPACK_SECURITY_ACKNOWLEDGED_LIFECYCLE_CATEGORY_UNUSED`.
- Stale lifecycle category scripts emit `TSPACK_SECURITY_ACKNOWLEDGED_LIFECYCLE_CATEGORY_STALE`.
- `tspack check --json` remains full-detail and includes lifecycle acknowledgment fields.
- Split manifests preserve root `Security.acknowledgedLifecycleCategories` with the rest of root security policy.
- Lifecycle execution remains blocked by default; category acknowledgment is audit metadata only.


## M49b outdated and dry-run UX normalization gate

- `tspack outdated` groups identical rows by default and shows declaring package count/list.
- `tspack outdated --per-package` restores expanded declaration-level output.
- Outdated grouping separates different kind, requested range, current, wanted, latest, status, and dependency identity.
- `tspack outdated --json` exposes grouped `entries` by default and declaration-level `dependencies` for compatibility; `--per-package --json` makes `entries` declaration-level.
- `tspack update --dry-run --json` uses a normalized `dryRun` object for full and targeted updates.
- `dryRun.enabled` is true for dry runs, `dryRun.changed` is a boolean, and `dryRun.summary` counts are numeric and never null.
- Workspace/non-registry outdated diagnostics use non-registry/not-applicable wording and code `TSPACK_OUTDATED_NON_REGISTRY_DEP`.
- Update resolver, lockfile, and write semantics remain unchanged.

## M50a declared update policy and dry-run report gate

- Root `<UpdatePolicy />` parses into manifest IR as `updatePolicy` and is preserved by split workspace parsing.
- Manifest validation rejects invalid policy kinds, invalid strategies, rolling rows without a level, levels on manual/pinned rows, invalid package scopes, and duplicate rows.
- `tspack outdated` evaluates registry dependencies against declared policy and reports `allowed`, `outside-policy-level`, `blocked-manual`, `pinned`, `unclassified`, and `not-applicable` statuses.
- Human outdated output shows policy status when a policy exists.
- `tspack outdated --json` includes policy fields for grouped and `--per-package` entries.
- Policy reporting is read-only: no lockfile, manifest, store, or `node_modules` mutation is allowed.
- M50a does not add `update --policy` mutation, automatic rolling updates, React coherence enforcement, dependency unification, or security gate enforcement.

## M50b policy-driven update planning gate

- `tspack update --policy --dry-run` succeeds as a read-only report and must not write `ts-lock.toml`, populate the store, materialize dependencies, or run lifecycle scripts.
- `tspack update --policy` without `--dry-run` fails clearly because policy-driven mutation is not implemented yet.
- Targeted policy planning is deferred: `tspack update <query> --policy --dry-run` fails with a targeted-policy unsupported diagnostic.
- Human output includes `Policy update plan (dry run)`, allowed/blocked/unclassified/not-applicable buckets when present, `security gates: not evaluated`, and `lockfile written: no`.
- JSON output includes normalized `dryRun` with `changed: false`, a `policyPlan` object, summary counts, `wouldUpdate`, `securityGatesEvaluated: false`, and `securityGateStatus: "not_evaluated"`.
- Missing `<UpdatePolicy />` is reportable and non-fatal when metadata resolution succeeds; `policyPresent` is false and registry candidates are unclassified.
- Normal `tspack update`, update dry-run, and `tspack outdated` semantics remain unchanged when `--policy` is absent.


## M50c policy update security gates

- `tspack update --policy --dry-run` evaluates existing TSPack security gates for policy update plan candidates and remains read-only.
- JSON reports `policyPlan.securityGatesEvaluated: true`, aggregate security status, candidate security statuses, `wouldApply`, `ready`, `securityBlocked`, and `reviewRequired`.
- Allowed candidates with `securityGateStatus: passed` are ready for future policy-driven application.
- Unacknowledged consumer-install lifecycle scripts block readiness.
- Unacknowledged maintainer-publish lifecycle scripts require review.
- Acknowledged exact lifecycle capabilities and matching lifecycle-category acknowledgments can pass; lifecycle execution remains blocked.
- The command does not mutate lockfiles, stores, or materialization and does not run lifecycle scripts or behavior fixtures.
- Normal update and outdated behavior remains unchanged.

## M50d rolling policy dogfood and closeout gate

- A realistic update-policy fixture exists under `examples/update-policy-notes` with app, library, and shared utility packages.
- The command matrix covers outdated human/JSON/per-package reports, policy dry-run human/JSON reports, check, check lifecycle details, doctor security human/JSON, and dry-run-only guardrails.
- No-mutation assertions pass for report/planning paths: lockfile bytes remain unchanged, store population is not caused by policy dry-run, materialization remains absent, and lifecycle scripts/behavior fixtures are not executed.
- Human outdated output remains grouped and readable for shared tool declarations while preserving `--per-package` expansion.
- Outdated JSON and policy dry-run JSON remain CI-stable with policy fields, compatibility declaration-level dependencies, non-null summary counts, stable candidate arrays, `effectiveAction`, `securityGateStatus`, and `securityGateReasons`.
- Security-gated plan statuses are verified for ready, review-capable, blocked-by-security, blocked-by-policy, unclassified, not-applicable, and noop buckets as applicable.
- Dry-run-only guardrails remain: `update --policy` without `--dry-run` fails, and targeted policy planning remains rejected.
- Dogfood documentation records current limitations, including no policy-driven mutation, no targeted policy planning, no external vulnerability feed, and no React single-version/coherence policy.

### M51a check --format release hardening

Release smoke for M51a must verify:

- The default Biome config ignores `.tspack/**`, `node_modules/**`, `dist/**`, and `tspack-artifacts/**`.
- `tspack init` generates a `biome.json` matching the same default ignore behavior.
- `tspack check --format` scopes Biome to source/project paths rather than the whole repository root.
- Generated store, artifact, and dist files do not fail format checks.
- Bad source formatting still fails with `TSPACK_FORMAT_CHECK_FAILED`.
- `tspack check --format --json` emits valid JSON-only stdout.
- Formatter output embedded in JSON diagnostics has no raw ANSI escape sequences.
- Top-level `tspack --help` advertises `check --format`.
- Missing formatter backend under `check --format` emits `TSPACK_FORMAT_BACKEND_MISSING` with the underlying Biome diagnostic in details.


## M53a parallel store population gate

The 0.1.0 release gate includes cold-update throughput hardening for `tspack update`:

- Store population is bounded-parallel only after deterministic resolution; resolver semantics and selected versions must not change.
- Lockfile bytes, package key sets, capability/security metadata, and update summary counts are deterministic between `TSPACK_STORE_JOBS=1` and `TSPACK_STORE_JOBS>1`.
- Targeted update preservation still passes with parallel store population.
- `update --dry-run` does not populate store artifacts.
- `update --policy --dry-run` does not populate store artifacts.
- Store writes are race-safe: unique temp paths are used, existing artifacts are reused, duplicate concurrent targets are accepted when the committed artifact already exists, and metadata remains deterministic.
- Invalid `TSPACK_STORE_JOBS` values fail clearly before store workers start.
- Performance benchmark or smoke commands are documented; CI must not require an exact wall-clock speedup.
- Workers must not print nondeterministically or run lifecycle scripts.

## v0.1.3 / M53c editor-tooling boundary gate

- `tspack init` generates `tsconfig.tspack.json` alongside `manifest.tsx` and `.tspack/types/tspack-manifest.d.ts`.
- Generated manifests resolve `tspack/manifest` through the project-local declaration surface.
- Manifest TSX typechecking uses `jsx: preserve` and does not require React or `react/jsx-runtime`.
- App `tsconfig.json` files generated or advised by init exclude TSPack-owned files: manifests, `*.xtest.tsx`, `.tspack/**`, `tspack-artifacts/**`, and `dist/**`.
- Existing app `tsconfig.json` handling is safe: init must not destructively rewrite it, and must print guidance when it leaves the file unchanged.
- Docs explain the TSPack-owned TSX boundary for manifests and native xTest files.

## v0.1.3 / M53e Windows local inspect gate

- Build the inspect bridge locally on Windows with `npm --prefix manifest-frontend run build` before running inspect smoke.
- `go run ./cmd/tspack inspect --help` works without requiring the bridge at runtime and prints inspect-specific help.
- `go run ./cmd/tspack inspect http://127.0.0.1:9 --json` emits valid JSON on stdout even on failure.
- Unreachable local URL smoke reports a stable browser/page diagnostic and does not print Linux-only `DISPLAY` / Xvfb guidance on Windows.
- Local server smoke covers:
  - `go run ./cmd/tspack inspect http://127.0.0.1:4171 --json`
  - `go run ./cmd/tspack inspect http://127.0.0.1:4171 --selector body --json`
- Browser path with spaces works through `--browser-path`, for example `C:\Program Files\Google\Chrome\Application\chrome.exe`.
- Windows Chromium missing-browser behavior is distinct from page-load failure:
  - missing browser runtime should report `TSPACK_INSPECT_BROWSER_NOT_FOUND` with Windows-appropriate remediation
  - unreachable page should report `TSPACK_INSPECT_PAGE_LOAD_FAILED`
- Windows Chromium fallback probes standard Edge/Chrome install locations when Playwright-managed Chromium is missing, but explicit `--browser-path` remains the override.
- Explicit `--browser vscode` uses Windows-aware executable discovery and must not fail only because PATH parsing assumed Unix separators.
- `npx --prefix manifest-frontend playwright install --list` or equivalent local evidence is recorded during release prep so browser availability is explicit.
- Full Phase 11 inspect expansion, deeper browser-matrix coverage, and broader editor-host workflows remain post-0.1.3 scope.

## M54a template init gate

- Built-in `static` template is loaded through the public template engine, not a special Go branch.
- Local template directories render with the same parser, validation, variables, overwrite behavior, and diagnostics.
- Concepts are parsed, syntax-validated, listed, and printed after init.
- Unsafe `from`/`to` paths are rejected.
- Existing files fail by default; `--force` overwrites only declared template files.
- Generated static projects include `tsconfig.tspack.json` and local `tspack/manifest` editor declarations.
- Generated static projects should pass `tspack check` and `tspack check --format` after the manifest frontend is available.
- Templates remain inert data and do not execute commands, install packages, or fetch remote code.
