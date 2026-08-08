# Aetheris P2-M1 stabilization

## Reproduced baseline

The installed `tspack v0.1.7` executable rejected Aetheris's browser target because
its intentionally empty declaration output (`types: ""`) was treated as an invalid
lock target. It also misread JSX text containing "Import" as a side-effect import
and generated a Biome 1 configuration that Biome 2 rejects.

The source tree at `v0.1.8` contains the corresponding focused fixes:

- side-effect import scanning requires whitespace immediately before its quoted
  specifier, so JSX text cannot become an import;
- app targets may omit declaration output through lock serialization and validation;
- the default Biome configuration uses Biome 2's `files.includes`,
  `experimentalScannerIgnores`, and `assist.actions.source.organizeImports` schema;
- format/lint/check derive source paths and exclude TSPack, package-manager, and
  build outputs.

Version splits remain warning-only diagnostics. Aetheris has 21 split package
families; these are resolver/peer/tooling families, not a TSPack error. Its 114
lifecycle scripts are acknowledged policy warnings and remain blocked from execution.

## Regression coverage

Focused coverage includes TSX import scanning, optional app target validation,
generated-path format scoping, and Biome 2 temporary-config shape. The last of those
also verifies cleanup of the temporary configuration file. Store coverage now also
exercises concurrent duplicate artifact publication; same-hash writes are serialized
without reducing concurrency across different artifacts.

## Aetheris acceptance

With a freshly built v0.1.8 CLI, `sync`, `check`, `check --format`, `run typecheck`,
`run test`, `run build`, and `run lint` pass for `aetheris.client`. The check is
formatter-stable after canonical Biome formatting; no generated file changes on a
subsequent sync/check cycle.

## Test performance

`go test ./...` completed successfully in 229.08 seconds once ignored build output
was absent. This workspace's untracked historical `dist` tree causes Go's recursive
`./...` discovery to spend minutes scanning generated output; it is not a package
test deadlock or network wait. The slow package is the CLI integration suite
(222.15 seconds); `internal/check` is next at 43.76 seconds. These are understood
integration-test costs, not hangs.
