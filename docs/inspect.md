# tspack inspect

> **Status: Experimental / unstable**
>
> `tspack inspect` is a structural UI inspection experiment. It is useful today, but it is not yet a stable public contract and CLI/API flags may still change as backend support is refined.

`tspack inspect <url>` performs structural UI inspection for rendered browser targets. Ordinary URL inspection uses the Playwright Chromium URL backend by default; it does not require VS Code or Code-family executable discovery.
M23 also supports inspecting declared run targets:
- `tspack inspect dev`
- `tspack inspect --run dev`

It is **not** screenshot matching, visual diffing, machine vision, component rendering, or dev-server startup.

## Current capability (experimental)

- structural DOM/layout/style/text/role extraction
- JSON and text output
- deterministic, versioned UI context bundles on stdout or through atomic file replacement
- selector filtering and hit-test support
- optional source hint extraction through `data-tspack-source`, `data-tspack-component`, and `data-tspack-symbol`; the VS Code extension can reveal existing workspace-contained files from those hints after safety validation
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
   - This is the default backend for URL targets when `--browser` is omitted or `--browser auto` is used.
   - Chromium discovery uses an explicit path when supplied, then Playwright's managed runtime, then a system Chromium/Chrome/Edge executable. No browser is downloaded implicitly. The WebKit aliases still require Playwright WebKit availability.
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

- `tspack inspect http://127.0.0.1:4171 --json`
- `tspack inspect http://127.0.0.1:4171 --selector main --json`
- `tspack inspect <url>` (Playwright Chromium backend by default; requires an available Playwright browser runtime)
- `tspack inspect <url> --browser webkit` (Playwright WebKit backend when WebKit is available)
- `tspack inspect <url> --host-path /path/to/chrome`
- `tspack inspect <url> --selector '[role="alert"]' --bundle`
- `tspack inspect <url> --bundle-output artifacts/ui-context.json`

### Chromium discovery and launch provenance

For Playwright Chromium inspection, TSPack tries the Playwright-managed
Chromium executable first. If that executable is missing, it discovers a
system browser without invoking a shell: `chromium`, `chromium-browser`,
`google-chrome`, `google-chrome-stable`, then `chrome` on PATH, plus the
existing Windows and macOS application locations. Explicit `--browser-path`
and `--host-path` values remain authoritative for their respective backends.

Inspect JSON reports the Chromium family separately from `launchBackend` and
`executable.source` (`playwright-managed`, `system`, `explicit`, or
`connected`). An executable path may appear in raw inspect JSON for diagnosis,
but deterministic context bundles retain only the source category and omit the
machine-specific path.

Direct host subprocesses receive the inherited process environment, with any
resolved `DISPLAY` override composed on top. TSPack does not serialize that
environment into inspect JSON, bundles, or diagnostics. On Linux, an existing
Xvfb session can therefore be qualified by setting `DISPLAY` before invoking
inspect; TSPack does not start or install Xvfb automatically.

### Windows local requirements

- `tspack inspect <url>` on Windows is intended for local desktop dogfooding, not Linux/Xvfb-style headless shims.
- Preferred happy path:
  - build the bridge first: `npm --prefix manifest-frontend run build`
  - ensure a Playwright Chromium runtime is installed: `npx --prefix manifest-frontend playwright install chromium`
- When Playwright Chromium is missing, the Windows Chromium URL backend now distinguishes a missing browser runtime from a page-load failure and keeps `--json` output parseable.
- Windows Chromium fallback probes these standard installed-browser paths when Playwright-managed Chromium is unavailable:
  - `%ProgramFiles(x86)%\Microsoft\Edge\Application\msedge.exe`
  - `%ProgramFiles%\Microsoft\Edge\Application\msedge.exe`
  - `%LocalAppData%\Microsoft\Edge\Application\msedge.exe`
  - `%ProgramFiles%\Google\Chrome\Application\chrome.exe`
  - `%ProgramFiles(x86)%\Google\Chrome\Application\chrome.exe`
  - `%LocalAppData%\Google\Chrome\Application\chrome.exe`
- You can always bypass discovery with `--browser-path "<full path to chrome-or-edge.exe>"`.
- Explicit `--browser vscode` uses Windows-friendly executable lookup and should no longer fail only because PATH parsing assumed Unix separators.
- Windows browser diagnostics must not suggest Linux-only `DISPLAY` / Xvfb setup for ordinary URL inspect.

VS Code/Electron app UI inspection:

- launch the app yourself with remote debugging:
  - `code --remote-debugging-port=9229`
- list targets with explicit CDP endpoint mode:
  - `tspack inspect --cdp http://127.0.0.1:9229 --list-targets`
- inspect an existing renderer target without navigating it when no URL is supplied:
  - `tspack inspect --cdp http://127.0.0.1:9229 --target 0 --selector .statusbar --json`
- Electron hosts can return `[]` from the HTTP `/json` endpoint even when renderer targets exist; `tspack inspect --list-targets` falls back to `Target.getTargets` over the browser CDP WebSocket so VS Code-like `vscode-file://vscode-app/.../workbench.html` targets can still be listed.


## VS Code source reveal from source hints

The TSPack Inspect VS Code extension includes **TSPack: Reveal Source for Selected Inspect Node**. After inspecting a CDP target and selecting a node with parsed `source.file` metadata, the command opens the hinted file at the one-based line/column from `data-tspack-source` after converting to VS Code's zero-based editor position.

Reveal is read-only and workspace-bound. The extension treats page-provided source hints as untrusted data, rejects absolute paths, URL-like schemes, parent traversal, and symlink escapes, requires the target file to already exist under the selected workspace folder, and never creates or mutates source files. Component and symbol hint fields are displayed as metadata only; they are not used for filesystem resolution.

## VS Code / Code-family current status

VS Code/Electron inspection is an explicit editor/host path, not the generic URL path. `TSPACK_INSPECT_VSCODE_NOT_FOUND` is reserved for explicit VS Code-family backend/probe modes such as `--browser vscode`; ordinary URL inspection should instead reach the Playwright URL backend and, if necessary, report a browser launch/load diagnostic.

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
- `--bundle` (write UI context JSON only to stdout)
- `--bundle-output <file>` (atomically replace the bundle and report success on stderr)
- `--run <target>`
- `--run-ready-timeout <seconds>` (default 30)

When `--json` is set and `--run` is used, progress and run-target logs go to stderr and JSON output remains on stdout.
When `--json` is set for direct URL/browser inspection, handled failures also emit valid JSON on stdout with structured diagnostics instead of mixing human error text into stdout.

## UI context bundles

`--bundle` builds a schema-version `1`, `tspack.uiContext` observation around
the inspect root. With `--selector`, that root is the selected semantic
subtree. The bundle includes the selected node, bounded ancestors/siblings/
children, browser and viewport provenance, hit-test evidence when present,
diagnostics, and source provenance. It contains no timestamp, process ID,
temporary profile path, environment value, or executable path, so repeated
runs under the same browser/layout environment are byte-comparable.

Source hints are untrusted page data. Before a bundle reads the bounded source
excerpt defined by the bundle schema, the shared validator rejects absolute
paths, URL schemes, parent traversal, missing files, and symlink escapes. A
rejected or malformed hint remains evidence with a validation error; it never
grants filesystem authority and never fails browser inspection itself.

For a RunTarget-backed CI flow, the following command owns process start, HTTP
readiness, inspection, bundle replacement, and process-tree cleanup:

```powershell
tspack inspect --run dev --selector '[role="alert"]' --bundle-output artifacts/alert-context.json
```

## Source hints

Inspect can report optional, page-provided source hints on individual nodes. The analyzer reads:

- `data-tspack-source`, in the form `<file>`, `<file>:<line>`, or `<file>:<line>:<column>`
- `data-tspack-component`
- `data-tspack-symbol`

When present, the node may include:

```json
{
  "source": {
    "raw": "src/components/Button.tsx:42:7",
    "file": "src/components/Button.tsx",
    "line": 42,
    "column": 7,
    "component": "Button",
    "symbol": "Button.Primary"
  }
}
```

Malformed source hints do not fail inspect. The bounded raw value is preserved with `source.parseError`. Inspect itself does not resolve or trust the path; only an explicit bundle/source consumer may validate it against a workspace before a bounded read. See [Inspect Source Mapping Design](inspect-source-mapping.md) for the staged source mapping strategy and security model.

## Inspect diagnostics

Target/input:
- `TSPACK_INSPECT_TARGET_REQUIRED`
- `TSPACK_INSPECT_INVALID_TARGET`

Browser/backend:
- `TSPACK_INSPECT_BROWSER_UNSUPPORTED`
- `TSPACK_INSPECT_BROWSER_NOT_FOUND`
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
- `TSPACK_INSPECT_BUNDLE_SELECTION_REQUIRED`
- `TSPACK_INSPECT_BUNDLE_TOO_LARGE`
- `TSPACK_INSPECT_INVALID_BUNDLE_OPTIONS`

## Manual probes

- `node scripts/inspect-host-probe.mjs`
- `node scripts/inspect-playwright-core-probe.mjs`
- `node scripts/inspect-platform-webview-probe.mjs`

## VS Code extension proof of concept

M39b adds a lightweight VS Code extension proof of concept under `extensions/tspack-vscode/`. The extension does not implement a separate inspector and does not attach to CDP directly. It shells out to the TSPack CLI so `tspack inspect` remains the source of truth for runtime inspection.

The extension supports the following proof-of-concept workflow:

1. Start VS Code, Chromium, or another Electron/Chromium host with an explicit remote debugging port, for example `code --remote-debugging-port=9229`.
2. Configure `tspack.inspect.cdpEndpoint` when the default `http://127.0.0.1:9229` is not the desired endpoint.
3. Run **TSPack: Inspect CDP Targets** to execute `tspack inspect --cdp <endpoint> --list-targets --json`.
4. Pick a target and let the extension execute `tspack inspect --cdp <endpoint> --target <index> --json`.
5. Review the runtime UI tree, selected node details, diagnostics, and copyable selected-node JSON inside VS Code.
6. Use **TSPack: Reveal Source for Selected Inspect Node** for read-only navigation from safe workspace-contained source hints.
7. Use **TSPack: Copy Selected Inspect Node LLM Context** to copy a deterministic selected-node bundle for future author/reviewer workflows.

The extension is intentionally scoped to observation: target listing, inspect tree rendering, selected-node details, JSON copy, safe source reveal, and context bundle copy. It does not implement visual editing, source mutation, LLM provider integration, screenshot/OCR/machine vision, framework adapters, or a Code-OSS fork.

See [Runtime-Grounded IDE Vision](runtime-grounded-ide.md) and [Runtime-Grounded IDE / Inspect Closeout](claude-fooding-runtime-grounded-ide.md) for the broader direction and closeout state.

## Native xTest helper

The same runtime inspect backend is available inside native xTest through the `inspect` helper namespace. Tests can call `inspect.url(url, options?)` for a browser-backed page or `inspect.cdp(endpoint, options?)` for an existing CDP target, then assert or snapshot the returned structured inspect JSON. Inspect calls are observations, not assertions, so an inspect-only fact still fails the native harness no-assertion check. Use `assert.inspect.*` for role, name, visibility, bounds, hit-test, and source-hint facts over already-collected JSON. See [native-test-harness.md](./native-test-harness.md#inspect-helpers) for examples and option types.

## Bridge build prerequisite

Development checkouts run `tspack inspect` through the manifest frontend inspect bridge. Build it with:

```sh
cd manifest-frontend && npm run build
```

The canonical artifact is `manifest-frontend/dist/inspect-cli.js`; Go CLI discovery also accepts the legacy `manifest-frontend/dist/src/inspect-cli.js` path. Browser backend availability is separate from bridge lookup: a missing Playwright browser executable may produce `TSPACK_INSPECT_BROWSER_LAUNCH_FAILED`, but that is not a bridge build failure.
