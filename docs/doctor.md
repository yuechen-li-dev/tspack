# `tspack doctor` (M27)

`doctor` reports local environment readiness for TSPack without mutating the project.

## Commands

- `tspack doctor`
- `tspack doctor format`
- `tspack doctor run`
- `tspack doctor inspect`
- `tspack doctor --json`
- `tspack doctor --root <path>`

## What doctor checks

- Project basics: root, `manifest.tsx`, `ts-lock.toml`, `node_modules`.
- Format/lint readiness: Biome backend resolution and config presence.
- Run readiness: runtime executables and declared run targets. The `system` runtime is built in and means “execute declared argv directly,” so it is available even though there is no `system` executable. `bun` and `deno` are reserved/future runtime backends and are reported as not applicable rather than warnings until implemented.
- Inspect readiness (**experimental**): environment suitability and explicit-backend requirements.
  - platform-webview candidate and session env checks (`DISPLAY`, `WAYLAND_DISPLAY`, `DBUS_SESSION_BUS_ADDRESS`).
  - CDP explicit endpoint policy readiness (`TSPACK_INSPECT_CDP_ENDPOINT` if set).
  - host-path explicit executable policy readiness (`TSPACK_INSPECT_HOST_PATH` if set).
  - Playwright Core provider resolution (`TSPACK_PLAYWRIGHT_CORE_PATH`, project-local, VS Code bundle candidates).
  - VS Code-family executable discovery (`code`, `code-insiders`, `code-oss`, `codium`, `vscodium`).

## What doctor does not do

- No installs/downloads (`npm`, `npx`, browser binaries, OS packages).
- No run-target startup.
- No port scanning.
- No auto-attachment to running apps.
- No package-manager mutation.
- No xTest bridge generation; use `tspack test --xtest-bridge <path>` when an explicit native bridge path is needed.

Scoped exit behavior:
- `tspack doctor format` exits nonzero when format-critical checks have errors.
- `tspack doctor run` exits nonzero when run-critical checks have errors.
- `tspack doctor` (all) and `tspack doctor inspect` remain informational by default.

Environment variables used only for readiness checks:
- `TSPACK_INSPECT_CDP_ENDPOINT`
- `TSPACK_INSPECT_HOST_PATH`
- `TSPACK_PLAYWRIGHT_CORE_PATH`

## Text details

Text output includes stable detail lines when checks provide structured details. Runtime checks include path/version when available. Run target checks include package, runtime, runtime availability, command first token, command availability, URL, ready kind, and ready path. Detail keys are sorted for deterministic output.

## Status meanings

- `ok`: ready/available.
- `warning`: optional capability missing or experimental limitations.
- `error`: required capability missing for selected scope.
- `not_applicable`: check is informational and requires explicit user input.
