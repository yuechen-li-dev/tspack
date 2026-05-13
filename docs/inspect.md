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
- existing inspect diagnostics remain unchanged (`...PAGE_LOAD_FAILED`, `...SELECTOR_NOT_FOUND`, etc).

## Notes

- Browser truth is still required (no static layout approximation in M21b).
- `auto` prefers VS Code/Electron when runnable, then falls back to Playwright Chromium.
- Backend/API-only projects still require a browser-rendered URL.
- Hit tests use `document.elementsFromPoint(x, y)` and report the top-to-bottom stack.
