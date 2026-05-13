# Doom tests

Doom tests are quarantined abnormal-termination tests for TSPack native harness.

- File kind: `*.prophecy.tsx`
- Elements: `<Prophecy name="...">`, `<Foretell reason="..." />`, optional `<CycleTime seconds={...} />`
- Command: `tspack doom`

`doom` runs each prophecy in a child process, writes artifacts under `.tspack/doom-artifacts` (or `--out`), and verifies envelope metadata.

Prophecies are not executed by `tspack test`.

Current first-pass limitations:
- No exact exit/signal matching.
- Single `<Foretell>` only.
- No worker pools.
