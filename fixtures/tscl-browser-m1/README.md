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

Serve `fixtures/tscl-browser-m1/dist/browser` over HTTP and open `index.html`.
The page displays a package-generated nanoid token and increments its Copeland
reducer-owned count after a real click.
