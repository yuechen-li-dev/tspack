# tspack inspect

> **Status: Experimental / unstable**
>
> `tspack inspect` is a structural UI inspection experiment. It is useful today, but it is not yet a stable public contract and CLI/API flags may still change as backend support is refined.

`tspack inspect <url>` performs structural UI inspection for rendered browser targets.
M23 also supports inspecting declared run targets:
- `tspack inspect dev`
- `tspack inspect --run dev`

It is **not** screenshot matching, visual diffing, machine vision, component rendering, or dev-server startup.

## Current capability (experimental)

- structural DOM/layout/style/text/role extraction
- JSON and text output
- selector filtering and hit-test support
- platform-webview backend scaffold/probe (intended future default)
- Playwright-backed inspection path for Chromium and WebKit
- explicit CDP endpoint inspection path
- explicit installed-host launch path
- Playwright Core provider probing/resolution, including VS Code-family bundled `playwright-core` candidates

## Backend taxonomy

The analyzer core is shared across backends. Backends are responsible for obtaining a browser-like execution context.

1. **Platform WebView backend (`platform-webview`)**
   - Intended future stable/default backend direction.
   - Uses the OS-provided webview engine where feasible:
     - Windows: WebView2
     - macOS: WKWebView / WebKit
     - Linux: WebKitGTK
   - Current M26 status: scaffold/probe only, still experimental.
   - Why this direction: reduce dependence on Playwright browser downloads and use platform-provided runtime engines.
   - Limitations: platform APIs differ, Linux often needs WebKitGTK + display/session runtime, and this is not a cross-browser conformance layer.

2. **CDP backend (`cdp`)**
   - Uses an explicit user-provided remote debugging endpoint (`--cdp`).
   - Preferred protocol seam for already-running Chromium/Electron hosts.
   - Does not scan local ports.
   - Does not silently attach to apps.

3. **Installed host backend (`host-path`)**
   - Uses explicit `--host-path` / `--browser-path` executable path.
   - Launches with remote debugging and a temporary profile, then inspects through CDP.
   - Best suited to Chrome/Chromium/Edge-style hosts.
   - Electron apps may not behave like generic URL browsers.
   - VS Code/Code-family launch remains environment-dependent.

4. **Playwright backends (`playwright-chromium`, `chromium`, `playwright-webkit`, `webkit`)**
   - Use Playwright automation.
   - May require matching Playwright browser binaries; the WebKit aliases require Playwright WebKit availability.
   - Useful for controlled browser inspection and CI when browsers are installed.
   - The browser-side analyzer is shared across Chromium and WebKit.

5. **Playwright Core provider**
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

- launch the app yourself with remote debugging:
  - `code --remote-debugging-port=9229`
- list targets with explicit CDP endpoint mode:
  - `tspack inspect --cdp http://127.0.0.1:9229 --list-targets`
- inspect an existing renderer target without navigating it when no URL is supplied:
  - `tspack inspect --cdp http://127.0.0.1:9229 --target 0 --selector .statusbar --json`
- Electron hosts can return `[]` from the HTTP `/json` endpoint even when renderer targets exist; `tspack inspect --list-targets` falls back to `Target.getTargets` over the browser CDP WebSocket so VS Code-like `vscode-file://vscode-app/.../workbench.html` targets can still be listed.

## VS Code / Code-family current status

- Code-family executables may be discoverable.
- On Linux, `/usr/bin/code` can be a wrapper while `/usr/share/code/code` is the Electron binary.
- The most reliable path is to launch Code yourself with `--remote-debugging-port=<port>` and inspect through `--cdp`; when no `--url` is provided, tspack attaches to the selected existing target and does not call `page.goto`.
- In containerized headless probe environments, wrapper-resolved and direct binary launch attempts may fail due missing dbus, missing X server/`DISPLAY`, and platform initialization failures. This remains an environment/runtime blocker, not proof that desktop-local VS Code inspection is impossible.
- Xvfb/host-display auto-shimming is deferred; current host/platform-webview diagnostics continue to report display/session blockers instead of silently starting a display server.

## Command

- `tspack inspect <url>`
- `tspack inspect --url <url>`
- `tspack inspect <target>`
- `tspack inspect --run <target>`

## Options

- `--browser auto|platform-webview|cdp|host-path|browser-path|playwright-chromium|chromium|playwright-webkit|webkit|vscode`
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
- `--run <target>`
- `--run-ready-timeout <seconds>` (default 30)

When `--json` is set and `--run` is used, progress and run-target logs go to stderr and JSON output remains on stdout.

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

Platform webview:
- `TSPACK_INSPECT_PLATFORM_WEBVIEW_UNAVAILABLE`
- `TSPACK_INSPECT_PLATFORM_WEBVIEW_INIT_FAILED`
- `TSPACK_INSPECT_PLATFORM_WEBVIEW_EVALUATION_FAILED`
- `TSPACK_INSPECT_PLATFORM_WEBVIEW_UNSUPPORTED_OS`

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
- `node scripts/inspect-platform-webview-probe.mjs`
