# TSPACK-TSCL-BROWSER-M1 fixture

This fixture is the browser counterpart to `fixtures/tscl-m1`.  It selects the
explicit `javascriptRuntime: "browser"` target, uses production Copeland ESM,
and imports the real `nanoid` npm package through TSPack's locked/materialized
graph.

```text
dotnet build ../Copeland/src/Copeland/Copeland.Cli/Copeland.Cli.csproj --no-restore
go run ./cmd/tspack update --root fixtures/tscl-browser-m1
go run ./cmd/tspack sync --root fixtures/tscl-browser-m1
go run ./cmd/tspack build --root fixtures/tscl-browser-m1 browser
```

For ordinary development, run:

```text
go run ./cmd/tspack run dev --root fixtures/tscl-browser-m1
```

TSPack builds the normal Copeland browser artifacts and supervises Vite at
`http://127.0.0.1:5173`. Editing `src/` recompiles Copeland and performs a
full browser reload after a successful build. A failed build keeps the prior
page available until the source is corrected.

This fixture also declares an owned ASP.NET development backend at
`http://127.0.0.1:5187`. Vite proxies `/api` (proved by `GET /api/status`) and
is configured to forward WebSocket route `/hub`; the fixture's M0 proof uses
HTTP only. Use `--env VITE_PORT=5190` when an isolated frontend port is needed.

Serving `fixtures/tscl-browser-m1/dist/browser` over static HTTP remains a
production-like compatibility smoke, not the preferred development command.
The page displays a package-generated nanoid token and increments its Copeland
reducer-owned count after a real click.
