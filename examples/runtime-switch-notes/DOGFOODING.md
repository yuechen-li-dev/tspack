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
| `go run ./cmd/tspack test --root examples/runtime-switch-notes` | pass | 4 native xTest facts passed. Required a native test bridge built by an ad hoc full `tsc`; see findings. |
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
| `cd manifest-frontend && npx tsc -p tsconfig.json` | fail but emitted bridge | Full frontend compile reported many existing TypeScript errors, but emitted `manifest-frontend/dist/src/native-test-cli.js`, which allowed `tspack test` to run. |

## Findings

### Bugs

- DOGFOOD_BUG: `internal/project` expected `manifest-frontend/dist/src/cli.js`, while `npm run build` emits `manifest-frontend/dist/cli.js`. A small fallback was added so project commands can use either layout.
- DOGFOOD_BUG: `tspack test` depends on `manifest-frontend/dist/src/native-test-cli.js`, but the documented `cd manifest-frontend && npm run build` excludes the native test bridge. A full `tsc -p tsconfig.json` emits it despite type errors, which is not a good release smoke path.

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
- DOGFOOD_DOCS: The native xTest bridge build story needs to say which command emits `native-test-cli.js` without relying on a failing full TypeScript compile.
- DOGFOOD_DOCS: App target `types: ""` and lockfile target validation disagree; docs or validation should clarify the intended lock representation.

### Nice surprises

- Missing Deno diagnostics for RunTargets were clear and actionable.
- Bun runtime execution did not use `bun run` or package scripts; it prefixed `bun` directly and satisfied stdout readiness.
- `pack --dry-run` and `pack --verify` gave concise, deterministic output and generated package metadata without package scripts.
- `why`, `why --json`, and `why --reverse` composed cleanly once a lock edge existed.
- Security doctor produced a clean no-lifecycle posture without requiring any lifecycle fixture.

## Required follow-up issues

- DOGFOOD_BUG: Align manifest frontend and native test bridge build outputs with CLI bridge discovery.
- DOGFOOD_FOOTGUN: Decide whether single-manifest packages should get package roots or whether `cwd: "package"` should be rejected/diagnosed earlier for rootless packages.
- DOGFOOD_FOOTGUN: Align app target empty `types` docs, IR, lockfile generation, and lockfile validation.
- DOGFOOD_DOCS: Update RunTarget runtime docs to consistently include `bun` and `deno`.
- DOGFOOD_FRICTION: Provide a documented way for examples to opt into the default Biome backend without package-manager pretzeling.

## Verdict

Outcome B: the sample is worth keeping as a future release smoke, M43b removed the update/store recursion blocker, and M43c removed the URL-inspect VS Code routing blocker. The dogfooding run still has concrete environment/build blockers such as missing Playwright browser executables, native test bridge build mismatch, and missing Biome backend.

The runtime-profile thesis mostly holds for the manifest line itself: `runtime="nodejs"` is visible in `doctor runtime`, does not delegate package-manager behavior, and explicit RunTargets keep Node.js/Bun/Deno execution separate. The one-line runtime profile story is less proven for the broader workflow because native test bridge build, inspect, and format/lint surfaces still have blockers or environment-dependent gaps.

Before release, the remaining highest-value fixes are the bridge/build output mismatch and the broader environment-dependent tool availability gaps. Once those are fixed, this example should become a useful end-to-end release smoke.
