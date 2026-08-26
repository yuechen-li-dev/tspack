# TSPack testing strategy

TSPack tests prove each contract at the cheapest layer that can actually prove
it. The goal is minimum sufficient evidence: a slower or more realistic test is
valuable only when it proves a fact that a cheaper test cannot.

## Test hierarchy

Use these lanes in order:

1. Pure/domain tests prove parsing-independent rules, classification, ordering,
   and transformations.
2. Application tests prove typed project operations, diagnostics, mutation
   authority, and before/after state.
3. Parser and renderer tests prove CLI requests, text, JSON, and exit mapping.
4. Filesystem/integration tests prove stores, locks, materialization, atomic
   replacement, and platform filesystem behavior.
5. Subprocess tests prove executable dispatch, process exit, real pipes,
   environment inheritance, signals, process groups, and file-lock contention.
6. Live/browser/network tests prove contracts with external services or
   specialized runtimes and remain visibly isolated from core semantics.

Use the lowest lane that proves the invariant. Moving a test downward must not
replace a process, filesystem, or network fact with a simulation that cannot
observe that fact.

## Process-test rule

A subprocess test requires a process-specific reason in its name or a nearby
comment. Valid reasons include executable dispatch, exit status, stdin/stdout
pipe behavior, environment or PATH inheritance, signals, child-process trees,
cross-process locks, and platform process behavior. Argument parsing, JSON
shape, text formatting, diagnostics, policy classification, and ordinary
project mutation are not process-specific.

Use `clitest.RunApp` by default. Use `clitest.RunProcess` or
`clitest.RunProcessInDir` explicitly for a real process contract. Process tests
share the package test binary and must not add per-test `go run` calls.

## Fixture rule

Use the smallest explicit fixture capable of proving the behavior. Prefer a
minimal manifest, a locked project, a materialized project, a registry fixture,
or a process fixture according to the lane being tested. Do not copy a complete
repository or hydrate a store when a small manifest and lock excerpt suffice.
Immutable fixture input may be reused; mutable workspaces may not be shared in a
way that creates ordering or parallel-test dependencies.

## Matrix rule

Test distinct semantic branches, not every cosmetic permutation of names,
paths, package spelling, or flag order. A table is justified when its rows reach
different branches. If several rows only re-prove the same branch, keep one
representative and improve its assertion.

## Integration rule

End-to-end tests prove that layers remain wired together. They do not repeat
every domain rule, JSON field, or diagnostic variation. The default confidence
spine should stay compact: one fresh lifecycle flow, one read-only lifecycle
matrix, representative resolution and security flows, one RunTarget process
flow, and specialized integration smoke where the integration itself matters.

Browser, Skyrim, registry, and live-service tests own only integration-specific
facts. Core semantics remain directly tested by their core or application
owner.

## Mutation evidence

Mutation authority belongs in application tests with before/after snapshots:

- update may write lock and store state;
- update and policy dry-runs do not write lock state;
- sync, check, pack, why, and outdated do not rewrite the lock;
- sync may hydrate and materialize only from existing lock truth;
- pack may write requested artifacts only.

One process smoke may prove CLI wiring. Do not spawn the binary once per row of
this matrix.

## Performance rule

Slow tests must justify their cost with unique evidence. When a package becomes
slow, inspect per-test timing and classify cost as semantic work, repeated
setup, process startup, compilation, filesystem copying, network wait,
sleep/polling, or fixture size. Remove avoidable cost at its source; do not hide
meaningful suites from the default validation path.

Prefer observable readiness (channels, sockets, files, or process state) over
arbitrary sleeps. Keep timing-based waits only when timing or timeout behavior
is the contract. Concurrency invariants normally need one focused proof and one
small end-to-end smoke, not equivalent contention matrices at every layer.

## Review checklist

For each new expensive test, answer:

- What unique correctness fact does it prove?
- Why can a cheaper lane not prove that fact?
- Is the fixture the smallest useful one?
- Does each table row reach a distinct semantic branch?
- Is a wait observing state rather than merely sleeping?
- If it launches TSPack, what process-specific contract requires that launch?
