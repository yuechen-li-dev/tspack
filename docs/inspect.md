# tspack inspect

`tspack inspect <url>` performs structural UI inspection against an already running HTTP/HTTPS URL.

It is **not** screenshot matching, visual diffing, machine vision, component rendering, or dev-server startup.

## Command

- `tspack inspect <url>`
- `tspack inspect --url <url>`

## Options

- `--browser chromium` (MVP backend)
- `--viewport WxH` (default `1440x900`)
- `--selector <css>` (first match is used)
- `--point x,y` (repeatable)
- `--json` (stdout JSON)
- `--out <file>` (JSON file)
- `--text <file>` (text file)

## Output

The inspect result includes DOM-derived structure, viewport-relative bounds (CSS px), visible text, role/name approximation, selected computed styles, hit-test stacks, and diagnostics.

## Notes

- Requires the JS inspect bridge and Playwright Chromium runtime.
- If browser binaries are missing, install them with `npx playwright install chromium`.
- Backend/API-only projects still require a browser-rendered URL.
- Hit tests use `document.elementsFromPoint(x, y)` and report the top-to-bottom stack.
- Future work may add `tspack run` target integration and non-Chromium backends.
