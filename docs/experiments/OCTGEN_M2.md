# OCTGEN-M2: TSPack external consumer and coordinated generation

> **Experimental, post-1.0 work.** OctGen is outside the Oct 1.0 compatibility
> promise. It is an explicit maintainer generator, never a Go compiler phase.

## Result

M2 supports **external consumption** of interpreted Oct through the public
`github.com/yuechen-li-dev/oct/experimental/octgen` package. Remote distribution
is deliberately unproven: the package is newer than an available public OctGen
revision and M2 does not publish, tag, or push one.

TSPack owns its concept decoder and Go-AST renderer in
[`tools/generate-concepts`](../../tools/generate-concepts). Oct owns the data
derivation. Upstream Oct owns only ordinary project loading, type checking,
interpretation, structural values, output confinement, stale checks, and staged
multi-output replacement. It contains no TSPack concept types or renderer.

## Specimen and computation

[`internal/concepts/concepts.oct`](../../internal/concepts/concepts.oct) replaces
the handwritten 15-entry built-in concept registry. Each record carries actual
TSPack facts: requirements, conflicts, project-kind compatibility, dependencies,
tools, peers, targets, run targets, update/security policy, package projection,
workspace, and pack data.

Oct derives default `Provides` and manifest concept identity from each name,
normalizes relationship lists by filtering duplicates, adds the default `npm`
dependency source, and expands three template composition cases into expected
dependencies, tools, peers, targets, run targets, projections, and pack state.
Go only validates and maps that computed structure to existing TSPack types.

Go generics can share operations over values but cannot materialize this committed
value-level catalog or the coordinated resolver expectations.

## Coordinated outputs

- `builtin_registry.generated.go` provides the production `builtinFragments`.
- `builtin_registry.generated_test.go` provides names and composition expectations.

The handwritten test loop calls independently implemented `BuildConceptIR`,
`ResolveWithRegistry`, and `Merge`; it verifies real lookup, requirement,
compatibility, dependency, target, projection, and pack behavior rather than only
comparing two generated lists.

## Local M2 invocation

The nested tool module currently uses this explicitly local, relative replacement:

```text
github.com/yuechen-li-dev/oct => ../../../oct
```

It is development wiring, not remote reproducibility. From the TSPack root:

```powershell
go -C tools/generate-concepts run . generate -input ../../internal/concepts/concepts.oct -output ../../internal/concepts/builtin_registry.generated.go -test-output ../../internal/concepts/builtin_registry.generated_test.go
go -C tools/generate-concepts run . check -input ../../internal/concepts/concepts.oct -output ../../internal/concepts/builtin_registry.generated.go -test-output ../../internal/concepts/builtin_registry.generated_test.go
```

After an explicit OctGen experimental release, TSPack should replace the local
replacement with a versioned `require github.com/yuechen-li-dev/oct vX.Y.Z` that
contains `experimental/octgen`. Upgrades remain an explicit maintainer action.

## Output, capability, and bootstrap boundary

```text
TSPack generator tool
  -> experimental/octgen (Oct parser/type checker/interpreter)
  -> concepts.oct
  -> TSPack decoder + Go-AST renderer
  -> two validated, staged committed Go files
  -> ordinary TSPack build/test
```

Every output path is supplied by the TSPack command, must be a distinct `.go`
file directly beside the generator, and is validated before rendering output is
replaced. Oct receives no filesystem, environment, process, network, random, or
clock authority. All artifacts are rendered and staged before replacement.

Multiple filesystem renames cannot be globally atomic. The public helper stages
temporary files, moves previous files to backups, installs all replacements, and
rolls back installed files/backups on an error. `check` only reads and reports
each stale destination.

The normal TSPack module does not import Oct. The generator is a nested,
maintainer-only Go module, so ordinary `go test ./internal/concepts` and
`go build ./cmd/tspack` consume the committed outputs without building or
executing OctGen. No bootstrap cycle exists: building Oct does not require
TSPack, and building normal TSPack packages does not require the tool.

## Measurements and limitations

| Measure | Result |
| --- | --- |
| Production declarations derived | 15 built-in concepts |
| Independently exercised coordinated cases | 3 (`static-app`, `react-app`, `react-library`) |
| Handwritten production descriptor lines removed | 26 physical lines (the former literals were unusually dense) |
| Handwritten test lines removed | 52 |
| Oct source | 281 lines |
| Generated production / test Go | 151 / 49 lines |
| TSPack-specific decoder and renderer | 692 lines |
| Reusable public OctGen seam | 266 implementation lines plus 73 focused-test lines |
| Cold local `go run ... check` | 10.306 s after `go clean -cache` |
| Warm local `go run ... check` | 560 ms |
| Determinism | byte-identical, covered by the tool test |

The generated Go intentionally remains verbose and reviewable. M2 adds a public
experimental structural-value/multi-output seam and a TSPack-local
decoder/renderer; it does not claim the infrastructure is amortized or remotely
installable yet.

Bounded verification passed:

- `go test -count=1 ./...` in `tools/generate-concepts`;
- `go test -count=1 ./internal/concepts` and `go build ./cmd/tspack`;
- fresh TSPack generator `check`;
- M0 and M1 OctGen `check` plus their targeted Go tests;
- relevant interpreted Oct enum-match and typed-empty-array corpus contracts;
- `git diff --check` in both repositories.

The normal TSPack concepts test was observed not to change either generated file.
The nested generator module's `go.sum` records the interpreter's Go dependencies;
the normal TSPack module neither imports nor downloads them for its bounded build.

The M0/M1 collection-transform friction recurs in `Normalize`, `WithDefaultSource`,
and composition collectors; this is **collection-transformation friction**. Repeated
record labels are **record-construction friction**. TSPack's detailed decoder is
**structured-decoding friction**. The local replacement is **tool-distribution
friction**. None yet justifies an Oct language change; the collection observation
also applies to Oct Make and ordinary Oct.

The next useful step is to stabilize an experimental OctGen distribution and public
tool contract, then repeat this boundary with one more independent consumer before
expanding the structured declaration vocabulary.
