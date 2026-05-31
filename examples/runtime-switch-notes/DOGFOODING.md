# Runtime Switch Notes Dogfooding Report

## Environment

- OS: Linux (`uname -a`: `Linux ... x86_64 GNU/Linux` in the container)
- Node.js: available at `/root/.nvm/versions/node/v24.15.0/bin/node`, version `v24.15.0`
- Bun: available at `/root/.local/share/mise/installs/bun/1.2.14/bin/bun`, version `1.2.14`
- Deno: not available on `PATH`
- Browser availability for inspect tests: Playwright package is present, but Chromium executables are not installed in the container; URL inspect now reaches the Playwright URL backend and fails with `TSPACK_INSPECT_BROWSER_LAUNCH_FAILED` instead of VS Code discovery
- Biome: not available in the example (`node_modules/@biomejs/biome` missing and no `biome` executable on `PATH`)

## Commands run

| command | result | notes |
| --- | --- | --- |
| `cd manifest-frontend && npm run build` | pass | Built the manifest frontend. npm warned about an unknown `http-proxy` env config. |
| `go run ./cmd/tspack check --root examples/runtime-switch-notes` | pass | Passed after adding `ts-lock.toml`. Before the lockfile existed, `check` only warned that the lockfile was missing. |
| `go run ./cmd/tspack check --root examples/runtime-switch-notes --format` | fail | `TSPACK_BIOME_BACKEND_NOT_FOUND`; useful diagnostic, but the example intentionally has no Biome tool dependency. |
| `go run ./cmd/tspack doctor runtime --root examples/runtime-switch-notes` | pass | Reported `runtime profile: ok`, selected `nodejs`, executable `node`, and no package-manager delegation. |
| `go run ./cmd/tspack doctor run --root examples/runtime-switch-notes` | pass | Node and Bun available; Deno missing is a warning. RunTargets are still reported as ok because missing Deno is environment availability, not manifest invalidity. |
| `go run ./cmd/tspack doctor security --root examples/runtime-switch-notes` | pass | Reported no lifecycle capabilities and blocked lifecycle execution posture. |
| `go run ./cmd/tspack doctor format --root examples/runtime-switch-notes` | fail | `biome backend missing`; also reported that no Biome config exists and TSPack defaults would be used. |
| `go run ./cmd/tspack run --root examples/runtime-switch-notes --list` | pass | Listed Node.js, Bun, and Deno targets with explicit runtimes, URLs, workspace cwd, and stdout readiness. |
| `go run ./cmd/tspack run --root examples/runtime-switch-notes node-server --once` | pass | Node server reached stdout readiness on port 4171 and stopped cleanly. |
| `go run ./cmd/tspack run --root examples/runtime-switch-notes bun-server --once` | pass | Bun server reached stdout readiness on port 4172 and stopped cleanly. |
| `go run ./cmd/tspack run --root examples/runtime-switch-notes deno-server --once` | expected fail | `TSPACK_RUN_RUNTIME_NOT_FOUND` clearly named runtime `deno`, executable `deno`, target `deno-server`, and install/change-runtime hint. |
| `go run ./cmd/tspack test --root examples/runtime-switch-notes` | pass | 4 native xTest facts passed after the documented `cd manifest-frontend && npm run build`; no full failing `tsc -p tsconfig.json` workaround was required. |
| `go run ./cmd/tspack test --root examples/runtime-switch-notes --compact` | pass | Compact output hid passing test details and kept the summary. |
| `go run ./cmd/tspack test --root examples/runtime-switch-notes --batch` | pass | Batch execution passed the same 4 tests. |
| `go run ./cmd/tspack pack --root examples/runtime-switch-notes --package @tspack-examples/runtime-switch-ui --dry-run` | pass | Planned README, LICENSE, CHANGELOG, `dist/ui/**`, and generated `package.json`. |
| `go run ./cmd/tspack pack --root examples/runtime-switch-notes --package @tspack-examples/runtime-switch-ui --verify` | pass | Produced and verified a deterministic package artifact; artifact was removed after the smoke. |
| `go run ./cmd/tspack why --root examples/runtime-switch-notes @tspack-examples/runtime-switch-ui` | pass | Explained the app package's workspace dependency and target reachability. |
| `go run ./cmd/tspack why --root examples/runtime-switch-notes @tspack-examples/runtime-switch-ui --json` | pass | JSON output matched the text explanation. |
| `go run ./cmd/tspack why --root examples/runtime-switch-notes --reverse @tspack-examples/runtime-switch-ui` | pass | Reverse why used the manually checked-in lock edge. |
| `go run ./cmd/tspack inspect http://127.0.0.1:4171 --json` | expected environment fail | Fixed in M43c: with the Node server running, URL inspect no longer reports `TSPACK_INSPECT_VSCODE_NOT_FOUND`; it reaches the Playwright URL backend and reports `TSPACK_INSPECT_BROWSER_LAUNCH_FAILED` because this container lacks Playwright Chromium executables. |
| `go run ./cmd/tspack inspect http://127.0.0.1:4171 --selector main --json` | expected environment fail | Fixed in M43c: selector routing also reaches the Playwright URL backend and fails with `TSPACK_INSPECT_BROWSER_LAUNCH_FAILED`, not the VS Code diagnostic. |
| `go run ./cmd/tspack format --root examples/runtime-switch-notes --check` | fail | `TSPACK_BIOME_BACKEND_NOT_FOUND`. |
| `go run ./cmd/tspack lint --root examples/runtime-switch-notes` | fail | `TSPACK_BIOME_BACKEND_NOT_FOUND`. |
| `go run ./cmd/tspack update --root examples/runtime-switch-notes` | pass | Fixed in M43b. Workspace dependency population completed without recursively copying `.tspack/store`; the generated lockfile now records the real workspace store hash. |

## Findings

### Bugs

- DOGFOOD_FIXED: M43d aligned bridge output and discovery. `cd manifest-frontend && npm run build` now emits `dist/cli.js`, `dist/inspect-cli.js`, and `dist/native-test-cli.js`, and Go bridge discovery prefers those current paths while still accepting legacy `dist/src/*.js` paths.
- DOGFOOD_FIXED: `tspack test --root examples/runtime-switch-notes`, `--compact`, and `--batch` pass after the documented build without running a failing full `tsc -p tsconfig.json` compile or hand-creating `dist/src`.

### Fixed in M43b

- DOGFOOD_BUG_FIXED: `tspack update --root examples/runtime-switch-notes` no longer recursively copies `.tspack/store` into itself for the sample's single-root workspace dependency. The M43b fix excludes TSPack-managed internal artifact directories while preserving real package content such as checked-in `dist/**` files.
- DOGFOOD_VERIFICATION: Re-ran `go run ./cmd/tspack update --root examples/runtime-switch-notes` and `go run ./cmd/tspack check --root examples/runtime-switch-notes`; both passed after `cd manifest-frontend && npm run build` made the manifest frontend CLI available.

### Fixed in M43c

- DOGFOOD_BUG_FIXED: `tspack inspect <url> --json` no longer requires VS Code discovery for ordinary URL targets. Auto URL routing selects the Playwright Chromium URL backend.
- DOGFOOD_VERIFICATION: Re-ran `go run ./cmd/tspack run --root examples/runtime-switch-notes node-server --once`; it reached stdout readiness on `http://127.0.0.1:4171`.
- DOGFOOD_VERIFICATION: Started `go run ./cmd/tspack run --root examples/runtime-switch-notes node-server` normally, then ran `go run ./cmd/tspack inspect http://127.0.0.1:4171 --json` and `go run ./cmd/tspack inspect http://127.0.0.1:4171 --selector main --json`. Both failed with `TSPACK_INSPECT_BROWSER_LAUNCH_FAILED` because Playwright Chromium executables are unavailable in this container; importantly, neither command reported `TSPACK_INSPECT_VSCODE_NOT_FOUND`.

### Footguns

- DOGFOOD_FOOTGUN: Single-manifest multi-package projects have no useful package root in this example. `cwd: "package"` made `doctor run` fail with `TSPACK_RUN_PACKAGE_ROOT_UNKNOWN`, so the sample uses explicit `cwd: "workspace"` instead.
- DOGFOOD_FOOTGUN: The lockfile validator requires non-empty target `types`, but app targets are documented as allowed to use `types: ""`. The sample gives the app a tiny `public/app.d.ts` to keep `check` and lock consistency green.
- DOGFOOD_FOOTGUN: No Biome backend means `check --format`, `doctor format`, `format --check`, and `lint` all fail. The diagnostics are clear, but a small example cannot exercise format/lint without adding a tool dependency and adding that dependency would expand the sample scope.

### Friction

- DOGFOOD_FRICTION: The two-package shape was useful for `why`, but awkward without split package directories. The project stayed single-root to keep scope small and documented the `cwd: "workspace"` compromise.
- DOGFOOD_FRICTION: Checked-in `dist/ui/**` is necessary because `pack` does not build. This is understandable, but examples need to say it plainly.
- DOGFOOD_FRICTION: Running commands through `go run ./cmd/tspack` from the repository root is more reliable than running from inside the example because development bridge discovery is mostly repository-layout-sensitive.

### Docs gaps

- DOGFOOD_DOCS: `docs/manifest.md` still says RunTarget `runtime` is `system` or `node` in one paragraph even though Bun and Deno are now supported elsewhere.
- DOGFOOD_DOCS: The bridge build story now names `cd manifest-frontend && npm run build` as the canonical prerequisite for manifest CLI, native xTest, and inspect bridges.
- DOGFOOD_DOCS: App target `types: ""` and lockfile target validation disagree; docs or validation should clarify the intended lock representation.

### Nice surprises

- Missing Deno diagnostics for RunTargets were clear and actionable.
- Bun runtime execution did not use `bun run` or package scripts; it prefixed `bun` directly and satisfied stdout readiness.
- `pack --dry-run` and `pack --verify` gave concise, deterministic output and generated package metadata without package scripts.
- `why`, `why --json`, and `why --reverse` composed cleanly once a lock edge existed.
- Security doctor produced a clean no-lifecycle posture without requiring any lifecycle fixture.

## Required follow-up issues

- DOGFOOD_FIXED: Manifest frontend, native xTest, and inspect bridge build outputs are aligned with Go CLI discovery.
- DOGFOOD_FOOTGUN: Decide whether single-manifest packages should get package roots or whether `cwd: "package"` should be rejected/diagnosed earlier for rootless packages.
- DOGFOOD_FOOTGUN: Align app target empty `types` docs, IR, lockfile generation, and lockfile validation.
- DOGFOOD_DOCS: Update RunTarget runtime docs to consistently include `bun` and `deno`.
- DOGFOOD_FRICTION: Provide a documented way for examples to opt into the default Biome backend without package-manager pretzeling.

## M43e rerun after M43b/M43c/M43d

### Environment

- Date: 2026-05-31 in the Codex container.
- Node.js: available at `/root/.nvm/versions/node/v24.15.0/bin/node`, version `v24.15.0`.
- Bun: available at `/root/.local/share/mise/installs/bun/1.2.14/bin/bun`, version `1.2.14`.
- Deno: not available on `PATH`.
- Playwright package: available through the manifest frontend dependency set, but Chromium browser executables are not installed in the container.
- Biome: no example-local `@biomejs/biome` install and no `biome` executable on `PATH`.

### Prep and repository checks

| command | result | notes |
| --- | --- | --- |
| `rm -rf examples/runtime-switch-notes/.tspack examples/runtime-switch-notes/tspack-artifacts tspack-artifacts` | pass | Removed generated state before the rerun. |
| `cd manifest-frontend && npm run build` | pass | Built the canonical manifest, native-test, and inspect bridge outputs. npm still warns about an unknown `http-proxy` env config. |
| `test -f manifest-frontend/dist/cli.js` | pass | Verified the manifest bridge emitted by the documented build. |
| `test -f manifest-frontend/dist/inspect-cli.js` | pass | Verified the inspect bridge emitted by the documented build. |
| `test -f manifest-frontend/dist/native-test-cli.js` | pass | Verified the native xTest bridge emitted by the documented build. |
| `cd manifest-frontend && npm run typecheck:manifest-api` | pass | Manifest API type surface still typechecks. |
| `cd manifest-frontend && npm test` | pass | 157 passed, 9 skipped. Skips were expected Playwright/host integration skips in this container. |
| `cd extensions/tspack-vscode && npm test` | pass | 35 extension tests passed. |
| `cd extensions/tspack-vscode && npm run compile` | pass | Extension TypeScript compile passed. |
| `go test ./...` | pass | Go test suite passed before the M43e fixes. After the M43e fixes, focused Go tests for the changed packages passed and the final full suite was rerun. |

### Dogfood matrix results

| command | result | notes |
| --- | --- | --- |
| `go run ./cmd/tspack update --root examples/runtime-switch-notes` | pass | With a clean `.tspack`, update completed with `lockfile diff: +0 -0`; no `file name too long` and no nested `.tspack/store/**/.tspack/store`. |
| `go run ./cmd/tspack update --root examples/runtime-switch-notes` | pass | A second update was idempotent with `lockfile diff: +0 -0`. M43e fixed a lockfile hash feedback loop found during the rerun. |
| `go run ./cmd/tspack check --root examples/runtime-switch-notes` | pass | Check passed after update. |
| `go run ./cmd/tspack check --root examples/runtime-switch-notes --format` | expected environment/tool fail | `TSPACK_BIOME_BACKEND_NOT_FOUND`; clear diagnostic, and the example intentionally does not add Biome just to turn this green. |
| `go run ./cmd/tspack doctor runtime --root examples/runtime-switch-notes` | pass | Selected runtime profile is `nodejs`; TSPack owns resolution, lockfile, materialization, lifecycle policy, security policy, check, and pack; `packageManagerDelegated: false`. |
| `go run ./cmd/tspack doctor run --root examples/runtime-switch-notes` | pass | Node and Bun available; Deno unavailable as a warning. Explicit RunTargets remain explicit and report their URLs/readiness. |
| `go run ./cmd/tspack doctor security --root examples/runtime-switch-notes` | pass | Security manifest loaded, no lifecycle capabilities recorded, lifecycle execution posture is blocked by default. |
| `go run ./cmd/tspack doctor format --root examples/runtime-switch-notes` | expected environment/tool fail | Biome backend missing; config warning says TSPack defaults would be used. |
| `go run ./cmd/tspack run --root examples/runtime-switch-notes --list` | pass | Lists Node.js, Bun, and Deno run targets with explicit runtimes. |
| `go run ./cmd/tspack run --root examples/runtime-switch-notes node-server --once` | pass | Node server reached stdout readiness at `http://127.0.0.1:4171`. |
| `go run ./cmd/tspack run --root examples/runtime-switch-notes bun-server --once` | pass | Bun server reached stdout readiness at `http://127.0.0.1:4172`. |
| `go run ./cmd/tspack run --root examples/runtime-switch-notes deno-server --once` | expected environment fail | `TSPACK_RUN_RUNTIME_NOT_FOUND` names runtime `deno`, executable `deno`, target `deno-server`, and gives an install/change-runtime hint. |
| `go run ./cmd/tspack test --root examples/runtime-switch-notes` | pass | 4 native xTest facts passed. M43e fixed duplicate discovery of tests copied into `.tspack/store`. |
| `go run ./cmd/tspack test --root examples/runtime-switch-notes --compact` | pass | Compact summary reports 4 passed. |
| `go run ./cmd/tspack test --root examples/runtime-switch-notes --batch` | pass | Batch mode reports the same 4 passing facts. |
| `go run ./cmd/tspack pack --root examples/runtime-switch-notes --package @tspack-examples/runtime-switch-ui --dry-run` | pass | Planned README, LICENSE, CHANGELOG, `dist/ui/**`, and generated package metadata. |
| `go run ./cmd/tspack pack --root examples/runtime-switch-notes --package @tspack-examples/runtime-switch-ui --verify` | pass | Produced and verified `tspack-examples-runtime-switch-ui-0.1.0.tgz`. |
| `go run ./cmd/tspack why --root examples/runtime-switch-notes @tspack-examples/runtime-switch-ui` | pass | Explains the app package's workspace dependency and target reachability. |
| `go run ./cmd/tspack why --root examples/runtime-switch-notes @tspack-examples/runtime-switch-ui --json` | pass | JSON output is clean and has no diagnostics. |
| `go run ./cmd/tspack why --root examples/runtime-switch-notes --reverse @tspack-examples/runtime-switch-ui` | pass | Reverse why reports the workspace UI package pulled by the app target. |
| `go run ./cmd/tspack inspect http://127.0.0.1:4171 --json` | expected environment fail | With the Node server kept running, URL inspect reaches the Playwright URL backend and fails with `TSPACK_INSPECT_BROWSER_LAUNCH_FAILED`; it does not report `TSPACK_INSPECT_VSCODE_NOT_FOUND`. |
| `go run ./cmd/tspack inspect http://127.0.0.1:4171 --selector main --json` | expected environment fail | Selector URL inspect has the same browser-executable limitation and no VS Code routing failure. |
| `go run ./cmd/tspack format --root examples/runtime-switch-notes --check` | expected environment/tool fail | `TSPACK_BIOME_BACKEND_NOT_FOUND`. |
| `go run ./cmd/tspack lint --root examples/runtime-switch-notes` | expected environment/tool fail | `TSPACK_BIOME_BACKEND_NOT_FOUND`. |

### Fixed blockers verified

- DOGFOOD_FIXED: M43b update/store recursion is verified. Update from a clean `.tspack` completed without recursive `.tspack/store` copying, without `file name too long`, and without nested `.tspack/store/**/.tspack/store` directories.
- DOGFOOD_FIXED: M43c URL inspect routing is verified. Ordinary URL inspect now reaches the Playwright backend; this environment's failure is `TSPACK_INSPECT_BROWSER_LAUNCH_FAILED`, not `TSPACK_INSPECT_VSCODE_NOT_FOUND`.
- DOGFOOD_FIXED: M43d bridge build/discovery is verified. The documented `cd manifest-frontend && npm run build` path emits `dist/cli.js`, `dist/inspect-cli.js`, and `dist/native-test-cli.js`, and native xTest runs through that bridge.
- DOGFOOD_FIXED: M43e removed duplicate native xTest discovery from generated `.tspack/store` and `tspack-artifacts` trees. The sample now reports 4 facts, not 8 duplicate facts after update.
- DOGFOOD_FIXED: M43e removed a workspace/path lock hash feedback loop by excluding generated `ts-lock.toml` from local source hashing/copying. Consecutive `update` runs now report `lockfile diff: +0 -0`.

### Remaining blockers and limitations

- DOGFOOD_ENV: Deno is not installed in this container. The failing Deno run command is an expected environment limitation with a clean `TSPACK_RUN_RUNTIME_NOT_FOUND` diagnostic.
- DOGFOOD_ENV: Playwright Chromium executables are not installed in this container. URL inspect cannot complete here, but routing reaches the correct backend.
- DOGFOOD_FRICTION: Biome is not installed for the sample. `check --format`, `doctor format`, `format --check`, and `lint` fail with clear `TSPACK_BIOME_BACKEND_NOT_FOUND` diagnostics. This remains acceptable for this sample because adding Biome would expand the example and package-manager surface.
- DOGFOOD_FOLLOWUP: The broader app target `types: ""` versus lockfile validator mismatch remains documented historical friction; the sample still carries `public/app.d.ts` to keep lock/check green.
- DOGFOOD_FOLLOWUP: Single-manifest multi-package projects still make `cwd: "package"` unhelpful for this shape, so the sample intentionally uses `cwd: "workspace"`.

### M43e verdict

Outcome A / LGTM for treating `examples/runtime-switch-notes` as a release-smoke candidate with documented environment skips/limitations.

The rerun verified the targeted M43b/M43c/M43d fixes and found two tightly scoped product footguns in the real smoke path: generated test trees were being discovered, and generated lockfile contents could feed back into workspace/path hashes. Both were fixed in M43e and verified before updating this report. The remaining red commands are environment/tool availability limitations, not product blockers for this sample.


## Verdict

Outcome A / LGTM for M43e: the sample is ready to be treated as a release-smoke candidate with documented environment skips. M43b removed the update/store recursion blocker, M43c removed the URL-inspect VS Code routing blocker, M43d removed the bridge build/discovery mismatch, and M43e removed duplicate generated-test discovery plus lockfile hash feedback discovered during the rerun. Remaining red commands are environment/tool availability items such as missing Deno, missing Playwright browser executables, and missing Biome backend.

The runtime-profile thesis mostly holds for the manifest line itself: `runtime="nodejs"` is visible in `doctor runtime`, does not delegate package-manager behavior, and explicit RunTargets keep Node.js/Bun/Deno execution separate. The one-line runtime profile story is less proven for the broader workflow because inspect and format/lint surfaces still have environment-dependent gaps.

Before release closeout, keep the environment skip conditions explicit rather than installing browsers or Biome implicitly. With those skips documented, this example is a useful end-to-end release smoke.
