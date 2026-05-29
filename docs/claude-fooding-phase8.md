# Claude-Fooding Phase 8 Format/Lint Closeout

Claude-fooding Phase 8 focused on TSPack's Biome-backed formatter and linter lifecycle helpers. The closeout state is **Success**: TSPack keeps delegation to Biome, documents the current user model, and release-gate coverage captures the remediated behavior without adding new formatter, linter, resolver, package-manager, or materializer features in this closeout milestone.

## Original findings

- Biome-backed delegation was architecturally correct; TSPack should not own a formatter or linter.
- The temporary config fallback was useful for projects without `biome.json` or `biome.jsonc`.
- Biome caught real code issues, including `useImportType` and `useButtonType` findings.
- Readable format output and Biome exit-code propagation were useful.
- The distinction between safe fixes and unsafe fixes was respected.
- `tspack format --check` passed a dead Biome 1.x `--check` flag.
- Backend resolution missed the direct `node_modules/@biomejs/biome/bin/biome` package binary.
- Executable-bit and root `.bin` materialization behavior needed regression verification.
- `tspack lint --fix` nonzero diagnostics were confusing when fixes were incomplete.
- There was no explicit unsafe-fix surface.
- The temporary default config was surprising because its style choices were not documented clearly enough.
- `tspack check --format` was desired for CI composition.

## Remediation summary

| Finding | Milestone | Fix | Status |
| --- | --- | --- | --- |
| `format --check` used a dead Biome 1.x flag and local backend resolution missed the direct package binary. | M38a | Run Biome format in non-write mode without forwarding Biome `--check`; resolve Biome through `node_modules/.bin/biome`, then `node_modules/@biomejs/biome/bin/biome`, then `PATH`; add executable-bit and root `.bin` regression coverage. | Done |
| Lint and format failures needed command-specific diagnostics while preserving Biome output. | M38b | Add `TSPACK_FORMAT_CHECK_FAILED`, `TSPACK_FORMAT_WRITE_FAILED`, `TSPACK_LINT_CHECK_FAILED`, and `TSPACK_LINT_FIX_INCOMPLETE`; keep generic backend failure for start, signal, and infrastructure failures; preserve stdout/stderr passthrough. | Done |
| The fallback Biome config was useful but surprising. | M38c | Document and report the opinionated temporary default config: tabs, 100 columns, double quotes, trailing commas, semicolons, arrow parentheses, organize imports, recommended lint rules, `useImportType` as an error, and unused variables/imports as warnings; suppress the default-config message when project config exists; report config source through doctor. | Done |
| Unsafe fixes needed an explicit surface. | M38d | Forward Biome unsafe fixes only for `tspack lint --fix --unsafe`; reject `--unsafe` without `--fix`; reject `--unsafe` for format. | Done |
| CI wanted normal check plus read-only format validation. | M38e | Add `tspack check --format`; report format failures as `TSPACK_FORMAT_CHECK_FAILED`; capture Biome output in JSON mode so `tspack check --format --json` keeps stdout parseable. | Done |

## Current format/lint model

- TSPack delegates formatting and linting to Biome rather than implementing its own formatter or linter.
- Biome backend resolution is local-first:
  1. `<root>/node_modules/.bin/biome`
  2. `<root>/node_modules/@biomejs/biome/bin/biome`
  3. `biome` from `PATH`
- A project `biome.json` or `biome.jsonc` wins and is used silently.
- If no project config exists, TSPack writes a temporary default Biome config for the command invocation.
- The temporary default config is not written to the project.
- When the temporary default config is used, stderr explains the default style summary.
- `tspack format` writes formatted files.
- `tspack format --check` performs read-only format validation.
- `tspack lint` is read-only.
- `tspack lint --fix` applies safe Biome fixes.
- `tspack lint --fix --unsafe` applies safe and unsafe Biome fixes after the user explicitly opts in.
- `tspack check --format` runs normal TSPack check plus read-only format validation.

## Current CI / development flow

```sh
tspack format --check
tspack format
tspack lint
tspack lint --fix
tspack lint --fix --unsafe
tspack check --format
tspack check --format --json
tspack doctor format
tspack how TSPACK_FORMAT_CHECK_FAILED
tspack how TSPACK_LINT_FIX_INCOMPLETE
```

Use `tspack format --check`, `tspack lint`, and `tspack check --format` as read-only gates. Use `tspack format`, `tspack lint --fix`, and carefully reviewed `tspack lint --fix --unsafe` during development when mutations are intended.

## Explicit non-goals / deferred work

Phase 8 does not implement:

- a TSPack-owned formatter or linter;
- `tspack check --lint`;
- formatting by default in `tspack check`;
- manifest-level format configuration;
- `tspack.json` configuration;
- Biome version management;
- npm/npx fallback;
- transitive bin exposure;
- materializer changes beyond regression coverage.
