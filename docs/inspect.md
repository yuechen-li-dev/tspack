# tspack inspect

`tspack inspect <url>` performs structural UI inspection against an already running HTTP/HTTPS URL.

It is **not** screenshot matching, visual diffing, machine vision, component rendering, or dev-server startup.

## Backend model

Inspect uses a backend abstraction and keeps one shared analyzer/formatter pipeline. Current backends:

- `vscode` (probe-backed VS Code/Electron runtime path)
- `playwright-chromium` (Playwright-managed Chromium)
- `host-path` (user-supplied Chromium/Electron-compatible executable; preferred)
- `browser-path` (compatibility alias for `host-path`)
- `chromium` (alias for `playwright-chromium`)
- `auto` (default: probe `vscode`, fallback to `playwright-chromium`)

## Command

- `tspack inspect <url>`
- `tspack inspect --url <url>`

## Options

- `--browser auto|vscode|playwright-chromium|chromium|browser-path|host-path`
- `--host-path <path>` (preferred explicit installed host launch)
- `--browser-path <path>` (compatibility alias)
- `--cdp <endpoint>` (explicit CDP backend, example: `http://127.0.0.1:9222`)
- `--list-targets` (list inspectable CDP targets without running analyzer)
- `--target <index-or-id>` (select specific target from list)
- `--target-url <substring>` (select first target by URL substring, ambiguous match fails)
- `--viewport WxH` (default `1440x900`)
- `--selector <css>` (first match is used)
- `--point x,y` (repeatable)
- `--json` (stdout JSON)
- `--out <file>` (JSON file)
- `--text <file>` (text file)

## Diagnostics

- `TSPACK_INSPECT_VSCODE_NOT_FOUND`
- `TSPACK_INSPECT_VSCODE_ELECTRON_NOT_USABLE`
- `TSPACK_INSPECT_BROWSER_PATH_NOT_FOUND`
- `TSPACK_INSPECT_CDP_ENDPOINT_REQUIRED`
- `TSPACK_INSPECT_CDP_ENDPOINT_INVALID`
- `TSPACK_INSPECT_CDP_CONNECT_FAILED`
- `TSPACK_INSPECT_CDP_TARGET_NOT_FOUND`
- `TSPACK_INSPECT_CDP_TARGET_AMBIGUOUS`
- `TSPACK_INSPECT_CDP_TARGET_UNSUPPORTED`
- `TSPACK_INSPECT_CDP_EVALUATION_FAILED`
- `TSPACK_INSPECT_INVALID_BACKEND_OPTIONS`
- existing inspect diagnostics remain unchanged (`...PAGE_LOAD_FAILED`, `...SELECTOR_NOT_FOUND`, etc).

## Notes

- Browser truth is still required (no static layout approximation in M21b).
- `auto` prefers VS Code/Electron when runnable, then falls back to Playwright Chromium.
- Backend/API-only projects still require a browser-rendered URL.
- Hit tests use `document.elementsFromPoint(x, y)` and report the top-to-bottom stack.

## CDP backend (installed/running host inspection)

The CDP backend is for inspecting an already-running Chromium-compatible host that exposes a remote debugging endpoint.

- Chrome/Chromium/Edge launched with remote debugging
- VS Code / Code Insiders / VSCodium launched with remote debugging
- Electron apps launched with remote debugging

Example launch:

- `chrome --remote-debugging-port=9222 --user-data-dir=/tmp/tspack-chrome`

Examples:

- `tspack inspect --cdp http://127.0.0.1:9222 --list-targets`
- `tspack inspect --cdp http://127.0.0.1:9222 --target 0 --json`
- `tspack inspect --cdp http://127.0.0.1:9222 --target-url localhost:5173 --json`
- `tspack inspect http://localhost:5173 --cdp http://127.0.0.1:9222 --json`

Scope/safety notes:

- CDP attachment is explicit (`--cdp` required).
- TSPack does not scan local ports and does not auto-attach to unrelated apps.


## Installed host backend

Use an explicit installed host runtime without Playwright-managed browser download:

- `tspack inspect <url> --host-path /path/to/chrome`
- `tspack inspect <url> --browser-path /path/to/chrome`

Behavior:

- TSPack validates the executable path, launches it with remote debugging and a temporary profile, then inspects through CDP.
- TSPack does not scan local ports and does not auto-attach to unrelated running apps.
- TSPack only launches the executable path you provide, or connects to an explicit `--cdp` endpoint.
- Code-family executables (`code`, `code-insiders`, `code-oss`, `codium`, `vscodium`) are valid explicit host candidates when they expose CDP.
- Not every Electron app supports arbitrary URL loading/debug flags; for already-running app targets prefer explicit CDP endpoint mode.

For already-running hosts:

- `tspack inspect --cdp http://127.0.0.1:9222 --list-targets`

Playwright remains an optional backend/fallback. `--host-path` is the preferred explicit installed-host path.

## Code-family host probe (manual)

Use the included probe script to discover/installability status and CDP readiness:

- `node scripts/inspect-host-probe.mjs`

Manual command equivalents:

- `command -v code || true`
- `command -v code-insiders || true`
- `command -v code-oss || true`
- `command -v codium || true`
- `command -v vscodium || true`
- `code --version || true`
- `code-oss --version || true`
- `codium --version || true`
- `vscodium --version || true`

Debian/Ubuntu notes:

- branded VS Code (`code`) usually comes from Microsoft's Linux repository / `.deb` setup.
- Code-OSS package names vary by distro and may not be present in default apt repositories.
- VSCodium commonly provides the `codium` executable via VSCodium distribution channels.


Container/root note:

- In root/container CI, some Electron hosts (including VS Code) may require sandbox overrides. Set `TSPACK_INSPECT_HOST_NO_SANDBOX=1` to enable `--no-sandbox --disable-gpu --disable-dev-shm-usage` for explicit host launch.
- This is only for explicit host launch and does not change CDP attach behavior.

VS Code/Electron host note:

- VS Code/Electron may expose workbench/devtools CDP targets without acting as a generic URL browser host.
- For generic URL inspection, Chrome/Chromium/Edge style hosts are usually more reliable.


Linux VS Code wrapper note:

- Code-family wrapper CLIs (for example `/usr/bin/code`) may not be the actual Electron binary used for process launch.
- TSPack probe logic resolves wrapper paths toward underlying binaries when available (for example `/usr/share/code/code` on Ubuntu/Debian).
- In this environment, probing both wrapper input and resolved binary was attempted; wrapper flags alone were not sufficient to expose `/json/version`.
