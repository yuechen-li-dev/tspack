# AGENTS.md

## Code readability rule

Do not write minified or overly compact code.

All TypeScript and Go should be formatted as ordinary readable source: multi-line functions, clear variable names, explicit branches where helpful, and small helpers for repeated logic. Avoid clever one-liners, nested ternaries, and dense chained expressions.

The preferred style is boring, explicit, and easy to review.

## Convergence rule

Every substantial task must end in exactly one of three states:

1. **Success**  
   The intended capability works in the real path and the real motivating case materially improves.

2. **Meaningful progression**  
   The capability is not complete, but one genuine blocker is removed and the next blocker is isolated with evidence.

3. **Honest stop**  
   Further work would require overbroad scope expansion, excessive debt, brittle patching, or tangled logic. Stop and report the reason with concrete evidence.

Do not continue producing patches once the work stops converging.

Do not confuse activity with progress.
A failed attempt is only acceptable if it leaves behind a narrower problem, stronger evidence, or a justified stop.

Any partial work must leave the codebase in a cleaner, more legible, and more diagnosable state than before.

## Architecture placement rules

- `cmd/` contains executable bootstrap only; command behavior belongs in `internal/cli`.
- CLI parsing and presentation do not own project, resolution, lockfile, store, materialization, security, or audit semantics.
- Core dependency direction flows from manifest and graph concepts toward project orchestration, then CLI; core packages never import CLI or integrations.
- Specialized browser, OS, ecosystem, deployment, and dogfood behavior lives behind explicit packages in `internal/integrations`.
- Manifest loading is an explicit application stage; do not hide frontend execution in a cheap constructor.
- Generated declarations and embedded assets have one canonical source and a drift check.
- CLI subprocess tests reuse the shared test binary. Do not add per-test `go run ./cmd/tspack` calls.
- New top-level `internal` packages require a clear, durable responsibility and dependency direction. Do not create generic `utils` packages.
- See `docs/dev/architecture.md` for the repository map and feature-placement guide.
