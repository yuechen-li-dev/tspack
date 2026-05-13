# tspack inspect

`tspack inspect <url>` performs structural UI inspection against an already running HTTP/HTTPS URL.

It is **not** screenshot matching, visual diffing, machine vision, component rendering, or dev-server startup.

## Backend model

Inspect uses a backend abstraction and keeps one shared analyzer/formatter pipeline. Current backends:

- `vscode` (probe-backed VS Code/Electron runtime path)
- `playwright-chromium` (Playwright-managed Chromium)
- `browser-path` (user-supplied Chromium/Electron executable)
- `chromium` (alias for `playwright-chromium`)
- `auto` (default: probe `vscode`, fallback to `playwright-chromium`)

## Command

- `tspack inspect <url>`
- `tspack inspect --url <url>`

## Options

- `--browser auto|vscode|playwright-chromium|chromium|browser-path`
- `--browser-path <path>`
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
