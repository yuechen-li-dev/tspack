# M68d runtime closeout

## Result

M68d removes the dominant repeated manifest bootstrap and materially reduces
CLI test work. It reaches meaningful progression rather than Outcome A because
the required count=1 full command remains dominated by a separately measured
196.767-second Go compile/link path before tests run.

## Runtime changes

- internal/manifestfrontend owns a bounded process-local Node worker.
- Requests use deterministic JSON lines, are serialized per worker, carry cwd
  and environment, and return request-scoped results and diagnostics.
- Older or fixture frontends that lack worker mode fall back to one-shot
  execution.
- Workers are keyed by Node identity, frontend artifact content hash, and the
  adjacent TypeScript package hash. Changed frontend or compiler artifacts
  start a new worker.
- Manifest results are never cached. Changed source is reevaluated by the same
  worker, so project and split-manifest file state cannot become stale.
- CLI TestMain closes workers. The test frontend script and IR storage are
  suite-owned; mutable IR remains request-scoped behind serialized CLI runs.
- Semantic audit, adoption, check/update, contract, why, and npm command tests
  use the in-process application path. Real process-tree, readiness, timeout,
  PATH, and executable contracts remain process tests.
- Three equivalent template declaration checks now share one real tsc project.
- The manifest frontend build uses sync.Once within the CLI suite.
- Node discovery caches successful resolution by operating system, cwd, PATH,
  and PATHEXT; changed resolution inputs cannot reuse a stale path.
- The runtime-switch matrix asserts all three workspace profiles but executes
  the identical explicit child-target matrix once.

The serialized CLI IO and typed-exit bridge was not a measured cost center.
Removing it would touch every legacy renderer while leaving the 197-second
compile/link blocker unchanged, so M68d keeps that bridge and reduces its test
surface by moving semantic suites to the existing in-process application path.

Representative structural counts changed as follows:

| Repeated work | Before | After |
| --- | ---: | ---: |
| Node/frontend starts for the three-profile runtime scenario | 3 | 1 |
| tsc starts for the equivalent template declaration matrix | 3 | 1 |
| generic TSPack process call sites in CLI tests | 120 | 76 |
| frontend builds within the CLI suite | repeated | 1 (`sync.Once`) |

The worker statistics API records starts, requests, one-shot fallbacks,
cumulative time, median, and maximum without enabling noisy output. Its
representative guard proves two manifest requests use one worker start.

## Precompiled fixture binary decision

A checked-in precompiled fixture executable is not justified. CLI subprocess
tests already reuse one suite-built TSPack binary, so another binary would not
remove the measured 196.767-second package compile/link floor. A reduced fake
binary would also stop proving the real CLI/process contract. Reusing the real
Node frontend process, the real compiler invocation, and the existing shared
CLI binary captures the useful part of that idea without binary artifacts,
platform matrices, or stale fixture behavior.

## Correctness and invalidation

Worker tests prove one bootstrap serves two requests, changed manifest bytes
produce changed IR, and a changed frontend artifact starts a replacement
worker. Real frontend build and tests retain cold compiler/frontend evidence.
The one-shot fallback retains older-bridge and executable-failure behavior.

## Security

Vitest moved from 2.1.9 to the patched 3.2 line for both frontend and VS Code
consumers. Lock regeneration intentionally updates the associated Vitest,
Vite, esbuild, Rollup, and support closure while retaining prior direct locked
versions for Biome, Playwright, TypeScript, Node types, and VS Code types. The
native OSV audit changed from six findings, including critical
GHSA-5xrq-8626-4rwp, to zero. Frontend tests pass on the upgraded runner.

## Timing summary

CLI baseline median was 69.789 seconds. Final dedicated runs were 44.776,
40.873, and 40.967 seconds (median 40.967), a 41 percent reduction. The
runtime-profile doctor test fell from 4.28
seconds to 0.06 seconds. The template compiler matrix fell from 4.16 seconds in
an intermediate profile to 0.57 seconds. Project remains approximately three
seconds: final runs were 3.479, 2.779, and 2.797 seconds (median 2.797).

The three final full runs passed in 230.312, 234.650, and 282.756 seconds
(median 234.650), a 26 percent reduction from the 316.138-second baseline.
A no-test, vet-disabled run reproduced 196.767 seconds. The next blocker is
therefore the Windows Go test-binary build/link and scanning path across
roughly forty packages. Merging production packages or hiding tests to reach a
target would weaken architecture or correctness.

## Development lanes

The default lane remains go test ./.... Focused package commands are the
interactive ownership lane on this host; frontend, focused race, VS Code,
compatibility, audit, and complete release commands are listed in
testing-strategy.md.

## M68 decision

M68 requires one final infrastructure/toolchain pass only if the project
requires count=1 go test ./... to meet the under-two-minute target on this
Windows host. The application and test-runtime foundation is otherwise clear:
manifest ownership, worker lifecycle, invalidation, compiler batching, cold
proofs, and security remediation are in place. Resolver, lock IR,
transactions, plugins, and embedding remain non-blocking future work.
