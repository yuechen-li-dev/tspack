# TSPACK-TSCL-M1 fixture

This fixture is the cross-repository Node proof. Its explicit `compilerPath`
targets the Debug `tscl` artifact built from the sibling Copeland checkout; the
path is a test-only discovery contract, not a distribution mechanism.

From the TSPack repository root:

```text
dotnet build ../Copeland/src/Copeland/Copeland.Cli/Copeland.Cli.csproj --no-restore
go run ./cmd/tspack build --root fixtures/tscl-m1
go run ./cmd/tspack run --root fixtures/tscl-m1 start
```

The expected output is `Hello, TSPack`.
