# M68e Go toolchain and test-build floor closeout

## Summary

The four-minute Windows Go loop was not primarily test execution, compilation,
linking, vet, or a Go cache failure. Repository-root `./...` package discovery
walked a generated `dist` tree containing approximately 651,000 files, 117,000
directories, and 64.7 GiB of release and benchmark workspaces. On this Windows
host, `go list -deps -test ./...` took 205.209 seconds before tests were relevant.

The architecture-complete roots `./cmd/... ./internal/... ./tools/...` produce
the same 41 Go packages without walking non-Go output. Their dependency listing
took 0.522 seconds. The final normal broad loop is approximately 19 seconds when
unchanged, approximately 43 seconds with every test forced to execute, and
52.406 seconds with an isolated cold build cache. No production package was
merged or fragmented for benchmark results.

## Baseline environment

- Git: clean `main` at `26019e7` before M68e changes.
- Go: `go1.27.0 windows/amd64`; module language version `go 1.25.0`.
- `GOOS=windows`, `GOARCH=amd64`, `CGO_ENABLED=1`, `GOTOOLCHAIN=auto`.
- `GOMAXPROCS` unset; runtime default on 16 logical processors.
- `GOCACHE=C:\Users\yuech\AppData\Local\go-build`.
- `GOMODCACHE=C:\Users\yuech\go\pkg\mod`.
- Windows product reported Windows 10 Pro, version 2009, build 26200.
- Repository, normal cache, and temporary paths are on the same NTFS NVMe SSD.
- Microsoft Defender antivirus, service, and real-time protection were enabled.
  The exact repository, cache, temp path, and `go.exe` were not reported as
  exclusions. Security settings were not changed.
- Baseline CLI: `tspack v0.1.8`, commit and built metadata unknown.

## Benchmark matrix

### Repository-root cached command

Before the discovery cause was isolated, three `go test ./...` runs measured:

| Run | Seconds | Cached test packages |
| --- | ---: | ---: |
| 1 | 281.300 | 0 |
| 2 | 212.230 | 32 |
| 3 | 279.120 | 33 |

After cache-churn fixes, population took 244.277 seconds; two fully cached runs
took 208.428 and 206.790 seconds. A fully cached `-x` run took 207.053 seconds
while invoking zero compiler, linker, or vet processes. The cache was working;
root package discovery was still walking generated output.

### Explicit-root developer command

After one 97.119-second population run in which only `internal/cli` executed,
three fully cached measurements were 19.590, 18.980, and 18.941 seconds:

| Lane | Min | Median | Max |
| --- | ---: | ---: | ---: |
| Cached explicit roots | 18.941s | 18.980s | 19.590s |

### Forced test execution

`go test ./cmd/... ./internal/... ./tools/... -count=1 -timeout 420s`:

| Run | Seconds |
| --- | ---: |
| 1 | 43.210 |
| 2 | 42.873 |
| 3 | 42.209 |

Min/median/max: 42.209/42.873/43.210 seconds.

The comparable M68d repository-root forced results were 230.312, 234.650, and
282.756 seconds, with a 234.650-second median. The command change removes the
generated-output discovery tax; it does not remove test coverage.

### Cold isolated build cache

With a new temporary `GOCACHE`, the explicit-root forced command completed in
52.406 seconds. The normal user cache was neither cleared nor modified by the
cold-lane setup.

## Exact `-count=1` behavior

Installed Go documentation describes `-count` as the number of executions for
each test and describes build outputs and successful package test results as
separate caches. Diagnostic execution confirmed the separation:

- a warm explicit-root `-count=1 -x` run took 42.489 seconds;
- it invoked zero compiler processes, 36 linker processes, and zero vet
  processes;
- all package tests executed;
- a normal unchanged run returned 36 cached test-package results.

Therefore `-count=1` disables successful test-result reuse for this command. It
does not disable the build cache. Cached package archives remain reusable, while
test binaries are relinked and test bodies rerun.

## Compile, link, and test execution

Warm `go test -c`, followed by direct execution of the compiled test binary:

| Package | Warm build + link | Direct test execution | Binary size |
| --- | ---: | ---: | ---: |
| `internal/cli` | 1.440s | 36.598s | 18.21 MiB |
| `internal/project` | 1.320s | 1.349s | 14.42 MiB |
| `internal/resolver` | 1.720s | 1.666s | 12.52 MiB |

Independent isolated-cache compile-plus-link measurements were 10.611 seconds
for CLI, 9.205 seconds for project, and 8.928 seconds for resolver. This proves
that neither compilation nor linking intrinsically requires minutes once the Go
command is pointed at the actual source roots.

## Test binary sizes and dependency fanout

Largest test binaries:

| Package | MiB |
| --- | ---: |
| `internal/cli` | 18.21 |
| `internal/project` | 14.42 |
| `internal/resolver` | 12.52 |
| `internal/installscript` | 10.36 |
| `internal/audit` | 6.75 |
| `internal/integrations/skyrim` | 6.72 |
| `internal/materialize` | 6.46 |
| `internal/why` | 6.45 |
| `internal/lockfile` | 6.45 |
| `internal/pack` | 6.41 |

Largest test dependency closures were CLI 266, project 245, resolver 227,
audit 216, and installscript 202 packages. The most widely present internal
packages were `diag` in 23 test closures, `pathutil` in 20, `manifest` in 19,
`graph` in 16, and `lockfile` in 11. These are consistent with their durable
cross-cutting/domain roles. No test-only import was found that justified a new
production package split.

Binary size broadly follows closure size, but warm links for the ten largest
binaries were only 0.80 to 1.72 seconds. Size did not correlate with the
205-second discovery floor.

## Generated and embedded assets

Normal `go:embed` inputs are small: manifest declarations are 7.7 and 12.9 KiB,
the browser runtime is 39.1 KiB, and 48 embedded template files total 90.2 KiB.
They do not materially explain normal link cost.

`internal/embeddedbridges/generated_assets.go` is approximately 45.8 MB because
it carries release bridge bundles, including TypeScript and browser tooling. It
is guarded by the `tspack_embedded_bridges` build tag and excluded from normal
tests. It remains in its narrow release owner. Its generator now compares bytes
and preserves the file and mtime when output is unchanged.

The manifest frontend build helper now avoids rebuilding `dist/cli.js` when the
compiled output is newer than all frontend source and configuration inputs. A
structural test proves unchanged output is reused and newer input requests a
build.

## Windows filesystem and security findings

The generated `dist` tree was the dominant repository-root traversal:

| Pattern | `go list -e` time |
| --- | ---: |
| `./dist/...` | 202.690s |
| `./extensions/...` | 1.060s |
| `./node_modules/...` | 0.450s |
| `./fixtures/...` | 0.200s |
| `./manifest-frontend/...` | 0.140s |

PowerShell counted about 651,229 files and 117,475 directories totaling 64.699
GiB in `dist`. Real-time scanning was active and may amplify metadata and
executable activity, but no Defender setting was disabled and no exclusion was
required for the improvement. The controlled same-machine comparison—205.209
seconds for root discovery versus 0.522 seconds for explicit roots—isolates the
avoidable repository traversal.

Developers may evaluate approved development-path exclusions under their own
organizational policy, but TSPack correctness and the recommended command do not
depend on one.

## Test helper builds and cache invalidation

Before M68e, tests contained three nested real Go invocations:

- CLI `TestMain` built one suite-owned TSPack binary;
- six migrate tests invoked `go run ./cmd/tspack` through one helper;
- a project-IR guard invoked `go run ./cmd/tspack help all`.

The migrate tests now reuse the suite-owned binary. The CLI-help guard moved to
the CLI package and uses the in-process app. The remaining single `go build` is
intentional: CLI process tests uniquely cover executable dispatch, exits, pipes,
environment inheritance, signals, and process trees.

Go cache diagnostics found a pack test writing an archive into shared fixture
input. The observed archive was always too new for safe result caching. It now
writes to `t.TempDir`. Concurrent CLI frontend generation also changed inputs
observed by project tests; freshness-aware build reuse removes that churn.

To verify ordinary invalidation, successful default commands were repeated
until every tested package reported `(cached)`. Source edits invalidated only
the relevant action and dependents. A representative Go-source edit caused the
CLI architecture/guardrail suite, which intentionally reads repository Go
sources, to rerun; the broad command completed in 72.618 seconds while 35 other
test packages were cached. Unchanged explicit-root reruns returned all 36 tested
packages from the result cache. Clearing only the test cache produced a
91.865-second population run followed by a 17.367-second cached rerun. No
changed-package mapper was added: Go's native cache plus focused package
commands already provide the needed behavior.

## Vet, race, and binary reuse

Vet-off repository-root measurements were 207.513, 207.309, and 225.294 seconds
(207.513-second median), indistinguishable from fully cached default root runs.
Vet was not the floor and remains enabled in the normal command.

Race validation remains a focused lane for resolver and other concurrency
owners. It is not part of every normal edit loop and is not weakened.

Compiled test-binary reuse is useful when repeatedly applying `-test.run` to an
unchanged expensive suite. For the normal workflow it adds artifact management
for little benefit: warm links are about one to two seconds, and Go already
reuses package compilation. No binaries are checked in.

## Build-graph changes and structural guardrails

No production package graph changed. Test ownership improved without collapsing
M68a-M68d boundaries.

Structural guards now prove:

- unchanged manifest frontend output does not request another build;
- newer frontend source does request a build;
- unchanged generated embedded-bridge bytes preserve mtime;
- changed generated bytes are written;
- pack fixture tests do not write artifacts into shared fixtures;
- test source contains only the suite-owned Go compiler invocation.

## Final developer and validation lanes

Focused owner:

```powershell
go test ./internal/cli
go test ./internal/project
go test ./internal/resolver
```

Broad developer correctness:

```powershell
go test ./cmd/... ./internal/... ./tools/...
```

Forced test execution:

```powershell
go test ./cmd/... ./internal/... ./tools/... -count=1 -timeout 420s
```

Cold toolchain/reproducibility:

```powershell
$oldGoCache = $env:GOCACHE
$env:GOCACHE = Join-Path $env:TEMP ("tspack-cold-" + [guid]::NewGuid())
go test ./cmd/... ./internal/... ./tools/... -count=1 -timeout 600s
$env:GOCACHE = $oldGoCache
```

Release validation combines the broad and forced/cold Go lanes with manifest
frontend build/typecheck/tests, VS Code compile/tests, compatibility drift,
audit, self-host, smoke, and platform-appropriate integration checks.

## M68 stopping decision

**M68 COMPLETE — Outcome A.**

The measured normal loop is interactive, cold validation is deliberate, Go
cache semantics are understood, and no P0 architectural friction remains.
Further graph-IR, plugin-architecture, or aesthetic package redesign belongs to
M69+.

## Final validation

Passed:

- broad explicit-root Go validation: 57.652 seconds after the final source
  changes; settled unchanged reruns were 18.753 and 18.807 seconds;
- forced explicit-root Go validation: 42.005 seconds;
- resolver race validation: 16.571 seconds, with package tests reporting
  3.750 seconds;
- manifest frontend build and manifest-API typecheck;
- manifest frontend Vitest: 23 files passed, 201 tests passed, 2 environment
  integrations skipped;
- VS Code compile and Vitest: 4 files and 35 tests passed;
- compatibility diff: all five generated surfaces up to date;
- audit: 117 locked npm packages scanned, zero known vulnerabilities;
- `--version`, help, check, why TypeScript, and outdated smoke commands;
- Windows PowerShell self-host command matrix: run-list, check, format check,
  security doctor, and policy dry-run all passed with no tracked mutation;
- `git diff --check`.

`check` continues to report the existing non-fatal `fsevents` multi-version and
lifecycle-policy summaries. Security execution remains blocked by policy; these
are not new M68e failures.

The self-host `go-test` RunTarget, Windows test matrix, development guide,
self-host guide, and current release checklists now use explicit Go roots.

## Deviations and deferred work

- The final root-pattern forced benchmark sequence was stopped when the user
  chose to clean the 64.7 GiB generated `dist` tree. M68d's three comparable
  root-pattern forced runs remain the pre-change baseline, and M68e separately
  measured root discovery, fully cached root behavior, three explicit-root
  forced runs, test-cache clearing, and isolated cold-cache behavior.
- `go test -json` was used only as a diagnostic because it executed package
  tests rather than reproducing the normal cached lane on this toolchain.
- No antivirus exclusion or security setting was changed.
- No changed-package engine, custom build system, package merge, release tag,
  publication, or checked-in test binary was added.
- Optional future work is limited to M69+ product architecture and any separate
  Go/Windows toolchain investigation. Neither blocks feature development.
