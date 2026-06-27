# Development notes

## Manifest frontend bridge build layout

Run the canonical bridge build before repository-root Go CLI dogfood or release smoke commands:

```sh
cd manifest-frontend && npm run build
```

That build emits the current bridge layout:

- `manifest-frontend/dist/cli.js` for manifest parsing and migration checks.
- `manifest-frontend/dist/native-test-cli.js` for native xTest, doom, benchmark, and artifact commands.
- `manifest-frontend/dist/inspect-cli.js` for `tspack inspect`.

Go bridge discovery prefers the current `dist/<bridge>.js` files and accepts legacy `dist/src/<bridge>.js` files for older dev flows. Do not rely on a failing full `tsc -p tsconfig.json` compile to create bridge artifacts.

## Windows local test path

For Windows triage and broad local validation, prefer a package-by-package Go sweep before a single broad `go test ./...` run:

```powershell
powershell -ExecutionPolicy Bypass -File .\tools\Run-GoTestMatrix.ps1
```

The matrix runner:

- lists `go list ./...` packages
- runs each package with `go test <pkg> -count=1 -timeout 180s -v`
- prints `PASS`, `FAIL`, or `TIMEOUT` with duration
- writes per-package stdout/stderr logs plus a JSON summary under `.tmp/go-test-matrix`

Recommended local tiers:

```powershell
# Fast Go
go test ./cmd/tspack ./internal/... -count=1

# Focused Windows-heavy coverage
go test ./cmd/tspack -run 'Run|Target|Tool|Bin|Shim|Path|Windows|Inspect|Test|Help|Init|Template|Ready' -count=1 -timeout 120s
go test ./internal/... -run 'Store|Materialize|Sync|Tar|Path|Symlink|Windows|Tool|Bin|Shim|ProjectIR|Ecosystem' -count=1 -timeout 120s

# Full Go
go test ./... -timeout 300s
```

Environment notes for Windows developers:

- `internal/installscript` covers `scripts/install.sh` and skips cleanly when `sh` is unavailable.
- Runtime-switch tests use Windows-friendly fake `bun`, `deno`, and `system` launchers and should not require Git Bash.
- Inspect and run-target tests should terminate child process trees cleanly on Windows rather than relying on Unix process-group semantics.
