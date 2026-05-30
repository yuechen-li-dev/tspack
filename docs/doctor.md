# `tspack doctor` (M27)

`doctor` reports local environment readiness for TSPack without mutating the project.

## Commands

- `tspack doctor`
- `tspack doctor format`
- `tspack doctor run`
- `tspack doctor inspect`
- `tspack doctor security`
- `tspack doctor security --json`
- `tspack doctor --json`
- `tspack doctor --root <path>`

## What doctor checks

- Project basics: root, `manifest.tsx`, `ts-lock.toml`, `node_modules`.
- Format/lint readiness: Biome backend resolution and config source. Backend details include the selected path, source (`local`, `direct-package`, or `path`), the root `.bin` candidate, and the direct `@biomejs/biome` package candidate. Config details report `configSource: project` with `configPath` when `biome.json`/`biome.jsonc` exists, or `configSource: tspack-default` with the default style summary when TSPack would use its temporary default config.
- Run readiness: runtime executables and declared run targets. Declared targets are reported with package-qualified IDs such as `runTarget:@scope/pkg:dev`. The `system` runtime is built in and means “execute declared argv directly,” so it is available even though there is no `system` executable. `bun` and `deno` are reserved/future runtime backends and are reported as not applicable rather than warnings until implemented.
- Inspect readiness (**experimental**): environment suitability and explicit-backend requirements.
  - platform-webview candidate and session env checks (`DISPLAY`, `WAYLAND_DISPLAY`, `DBUS_SESSION_BUS_ADDRESS`).
  - CDP explicit endpoint policy readiness (`TSPACK_INSPECT_CDP_ENDPOINT` if set).
  - host-path explicit executable policy readiness (`TSPACK_INSPECT_HOST_PATH` if set).
  - Playwright Core provider resolution (`TSPACK_PLAYWRIGHT_CORE_PATH`, project-local, VS Code bundle candidates).
  - VS Code-family executable discovery (`code`, `code-insiders`, `code-oss`, `codium`, `vscodium`).
- Security audit summary: lifecycle capability counts, per-capability acknowledgment status, stale or unused lifecycle acknowledgments, pulled-by paths when lock edges are available, and the non-execution posture for lifecycle scripts.

## What doctor does not do

- No installs/downloads (`npm`, `npx`, browser binaries, OS packages).
- No run-target startup.
- No port scanning.
- No auto-attachment to running apps.
- No package-manager mutation.
- No xTest bridge generation; use `tspack test --xtest-bridge <path>` when an explicit native bridge path is needed.
- No lifecycle script execution or lifecycle behavior probing; `doctor security` only reads manifest policy and lockfile metadata.
- No vulnerability database scanning, `npm audit`, registry checks, approval policy generation, rebuilds, jailed builds, or package-manager mutation.

Scoped exit behavior:
- `tspack doctor format` exits nonzero when format-critical checks have errors.
- `tspack doctor run` exits nonzero when run-critical checks have errors.
- `tspack doctor security` exits nonzero only for error-level security findings. Warning-only security findings, such as unacknowledged lifecycle scripts, exit `0`.
- `tspack doctor` (all) and `tspack doctor inspect` remain informational by default.

Environment variables used only for readiness checks:
- `TSPACK_INSPECT_CDP_ENDPOINT`
- `TSPACK_INSPECT_HOST_PATH`
- `TSPACK_PLAYWRIGHT_CORE_PATH`

## Text details

Text output includes stable detail lines when checks provide structured details. Runtime checks include path/version when available. Run target checks include package, runtime, runtime availability, command first token, command availability, URL, ready kind, and kind-specific readiness details such as ready path, host, port, pattern, and stream. Detail keys are sorted for deterministic output.

## Status meanings

- `ok`: ready/available.
- `warning`: optional capability missing or experimental limitations.
- `error`: required capability missing for selected scope.
- `not_applicable`: check is informational and requires explicit user input.

## Run target cwd details

`tspack doctor run` reports each RunTarget effective `cwd`, resolved `cwdPath`, `commandFirstToken`, and command/runtime availability. For `cwd: "package"`, doctor also reports `packageRoot` when it can be resolved.

## Run environment overlays

`tspack doctor run` inspects declared RunTargets and runtime availability only. It does not accept or evaluate CLI `--env` overlays, because those apply only when `tspack run` or `tspack inspect --run` starts a child process.

## Security scope

`tspack doctor security` is a read-only lifecycle capability audit view for the current project. It is part of the Phase 7 security/policy closeout documented in `docs/claude-fooding-phase7.md`. It loads manifest security acknowledgments and `ts-lock.toml`, then reports a `Security` section in text or JSON. The all-scope `tspack doctor` output also includes this concise `Security` section.

The lifecycle summary reports:

- total lifecycle capabilities;
- acknowledged and unacknowledged capability counts;
- stale acknowledgment count, where the manifest package/kind/script matches but the command has drifted;
- unused acknowledgment count, where a manifest acknowledgment is not present in the lockfile;
- packages with lifecycle scripts count;
- behavior fixture/report present, missing, and invalid counts when acknowledgments link evidence.

Each lifecycle capability check includes the lock package ID, script, command, `execution: blocked`, whether it is acknowledged, the acknowledgment reason when present, behavior fixture/report paths and statuses when present, and deterministic `pulledBy` path strings when lockfile edges make them available. Exact package/kind/script/command matches are `ok`; unacknowledged or stale capabilities are warnings. Unused acknowledgments are separate warning checks. Duplicate or invalid manifest policy remains manifest validation and is surfaced as an error diagnostic check if the manifest frontend reports it.

Doctor security never runs behavior fixtures or probes. If `ts-lock.toml` is missing, doctor security still validates manifest evidence paths, warns that locked lifecycle capabilities cannot be audited, and recommends `tspack update` to resolve and record package capabilities. It does not fabricate zero capabilities, and it does not report unused acknowledgments because there is no lock graph to compare against. If a lockfile exists and records no lifecycle capabilities, the lifecycle summary is `ok` with zero counts.

`--json` writes only parseable JSON to stdout, uses two-space indentation, appends a trailing newline, and keeps check ordering deterministic. Human text uses the same sections, checks, statuses, and sorted detail keys.

## `tspack doctor runtime`

`doctor runtime` reports the selected workspace runtime profile without changing command behavior.

Text output includes a `Runtime profile` section with:

- `selected`: `nodejs`, `bun`, or `deno`
- `executable`: `node`, `bun`, or `deno`
- `available`: whether the selected executable is found on `PATH`
- `status`: `supported` for `nodejs`, `experimental` for `bun` and `deno`
- lifecycle ownership details showing TSPack as the owner of dependency resolution, `ts-lock.toml`, materialization, security policy, and lifecycle policy

`doctor runtime --json` emits the same section as structured doctor checks. The runtime profile check includes `selected`, `executable`, `available`, `status`, `lifecycleOwner: "tspack"`, and `packageManagerDelegated: false`.

Only the selected runtime profile is checked prominently. Missing non-selected Bun or Deno executables do not create warning noise.
