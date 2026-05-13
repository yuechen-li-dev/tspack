# tspack inspect

> **Status: Experimental / unstable**
>
> `tspack inspect` is a structural UI inspection experiment. It is useful today, but it is not yet a stable public contract and CLI/API flags may still change as backend support is refined.

`tspack inspect <url>` performs structural UI inspection for rendered browser targets.

It is **not** screenshot matching, visual diffing, machine vision, component rendering, or dev-server startup.

## Current capability (experimental)

- structural DOM/layout/style/text/role extraction
- JSON and text output
- selector filtering and hit-test support
- Playwright-backed inspection path
- explicit CDP endpoint inspection path
- explicit installed-host launch path
- Playwright Core provider probing/resolution, including VS Code-family bundled `playwright-core` candidates

## Backend taxonomy

The analyzer core is shared across backends. Backends are responsible for obtaining a browser-like execution context.

1. **Playwright backend**
   - Uses Playwright automation.
   - May require Playwright browser binaries.
   - Useful for controlled browser inspection and CI when browsers are installed.

2. **CDP backend**
   - Uses an explicit user-provided remote debugging endpoint (`--cdp`).
   - Preferred protocol seam for already-running Chromium/Electron hosts.
   - Does not scan local ports.
   - Does not silently attach to apps.

3. **Installed host backend**
   - Uses explicit `--host-path` / `--browser-path` executable path.
   - Launches with remote debugging and a temporary profile, then inspects through CDP.
   - Best suited to Chrome/Chromium/Edge-style hosts.
   - Electron apps may not behave like generic URL browsers.
   - VS Code/Code-family launch remains environment-dependent.

4. **Playwright Core provider**
   - Automation client provider, not a browser runtime.
   - Resolution order:
     - `TSPACK_PLAYWRIGHT_CORE_PATH`
     - project-local `node_modules/playwright-core` / `node_modules/playwright`
     - VS Code-family bundled `playwright-core` candidates
   - Distinction:
     - `playwright-core` = client library
     - Playwright browser download = browser runtime artifact
   - VS Code-bundled `playwright-core` use is opportunistic, not a stable external VS Code contract.

## Safety policy

- TSPack never silently scans local ports.
- TSPack never silently attaches to arbitrary running apps.
- You must provide one explicit input mode:
  - URL (`tspack inspect <url>`),
  - CDP endpoint (`--cdp`), or
  - host executable path (`--host-path`, with `--browser-path` alias).

## Recommended usage today

Most reliable (already-running host with explicit endpoint):

- `tspack inspect --cdp http://127.0.0.1:9222 --list-targets`
- `tspack inspect --cdp http://127.0.0.1:9222 --target 0 --json`

URL inspection:

- `tspack inspect <url>` (Playwright backend; requires available browser runtime)
- `tspack inspect <url> --host-path /path/to/chrome`

VS Code/Electron app UI inspection:

- launch the app yourself with remote debugging
- then use explicit `--cdp` endpoint mode

## VS Code / Code-family current status

- Code-family executables may be discoverable.
- On Linux, `/usr/bin/code` can be a wrapper while `/usr/share/code/code` is the Electron binary.
- In this container probe environment, both wrapper-resolved and direct binary launch attempts failed due missing dbus, missing X server/`DISPLAY`, and platform initialization/SIGSEGV failures.
- This is an environment/runtime blocker in containerized headless probes, not proof that desktop-local VS Code inspection is impossible.
- Additional desktop-local verification is still needed.

## Command

- `tspack inspect <url>`
- `tspack inspect --url <url>`

## Options

- `--browser auto|vscode|playwright-chromium|chromium|browser-path|host-path`
- `--host-path <path>` (preferred)
- `--browser-path <path>` (compatibility alias)
- `--cdp <endpoint>`
- `--list-targets`
- `--target <index-or-id>`
- `--target-url <substring>`
- `--viewport WxH`
- `--selector <css>`
- `--point x,y`
- `--json`
- `--out <file>`
- `--text <file>`

## Inspect diagnostics

Target/input:
- `TSPACK_INSPECT_TARGET_REQUIRED`
- `TSPACK_INSPECT_INVALID_TARGET`

Browser/backend:
- `TSPACK_INSPECT_BROWSER_UNSUPPORTED`
- `TSPACK_INSPECT_BROWSER_LAUNCH_FAILED`
- `TSPACK_INSPECT_BRIDGE_MISSING`
- `TSPACK_INSPECT_FAILED`
- `TSPACK_INSPECT_INVALID_BACKEND_OPTIONS`

CDP:
- `TSPACK_INSPECT_CDP_ENDPOINT_REQUIRED`
- `TSPACK_INSPECT_CDP_ENDPOINT_INVALID`
- `TSPACK_INSPECT_CDP_CONNECT_FAILED`
- `TSPACK_INSPECT_CDP_TARGET_NOT_FOUND`
- `TSPACK_INSPECT_CDP_TARGET_AMBIGUOUS`
- `TSPACK_INSPECT_CDP_TARGET_UNSUPPORTED`
- `TSPACK_INSPECT_CDP_EVALUATION_FAILED`

Installed host:
- `TSPACK_INSPECT_HOST_PATH_NOT_FOUND`
- `TSPACK_INSPECT_HOST_PATH_INVALID`
- `TSPACK_INSPECT_HOST_LAUNCH_FAILED`
- `TSPACK_INSPECT_HOST_CDP_ENDPOINT_FAILED`
- `TSPACK_INSPECT_HOST_CLEANUP_FAILED`

Playwright Core provider:
- `TSPACK_INSPECT_PLAYWRIGHT_CORE_NOT_FOUND`
- `TSPACK_INSPECT_PLAYWRIGHT_CORE_LOAD_FAILED`

Page/analyzer:
- `TSPACK_INSPECT_PAGE_LOAD_FAILED`
- `TSPACK_INSPECT_ANALYSIS_FAILED`
- `TSPACK_INSPECT_SELECTOR_NOT_FOUND`
- `TSPACK_INSPECT_INVALID_VIEWPORT`
- `TSPACK_INSPECT_INVALID_POINT`

## Manual probes

- `node scripts/inspect-host-probe.mjs`
- `node scripts/inspect-playwright-core-probe.mjs`
