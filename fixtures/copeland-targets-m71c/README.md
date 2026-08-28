# Copeland backend targets M71c

The same Copeland TS source is selected by four compiler targets. `tsconfig.tsx`
owns backend/runtime semantics; `manifest.tsx` selects the target and describes
the generic artifact needed by the TSPack graph.

On Windows x64, after building the sibling Copeland CLI:

```powershell
go run ./cmd/tspack build app-js --root fixtures/copeland-targets-m71c
go run ./cmd/tspack build app-clr --root fixtures/copeland-targets-m71c
go run ./cmd/tspack build app-native --root fixtures/copeland-targets-m71c
go run ./cmd/tspack run run-js --root fixtures/copeland-targets-m71c
go run ./cmd/tspack run run-clr --root fixtures/copeland-targets-m71c
go run ./cmd/tspack run run-native --root fixtures/copeland-targets-m71c
```

The expected output in all three executable lanes is `Copeland M71c parity`.
`app-wasm` uses the normal .NET `browser-wasm` publish path and reports the
missing `wasm-tools` workload honestly when it is unavailable.
