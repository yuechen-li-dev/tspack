# Format and lint commands

TSPack provides Biome-backed code formatting and linting orchestration.

- `tspack format [paths...] [--root .] [--check]`
- `tspack lint [paths...] [--root .] [--fix] [--unsafe]`
- `tspack check [--root .] [--manifest <path>] [--json] --format`

## Backend resolution

TSPack resolves the Biome backend in this order:

1. `<root>/node_modules/.bin/biome`
2. `<root>/node_modules/@biomejs/biome/bin/biome`
3. `biome` from `PATH`

The direct package fallback supports strict TSPack materialization layouts where the package binary exists even if a package-manager-style shim is unavailable. TSPack does not use `npm run`, `npx`, `bunx`, `pnpm dlx`, or `yarn dlx`.

If Biome is missing, TSPack emits `TSPACK_BIOME_BACKEND_NOT_FOUND`.

## Config behavior

- If `biome.json` or `biome.jsonc` exists in project root, Biome uses that project config and TSPack does not print any default-config status message.
- If neither config file exists, TSPack generates an opinionated temporary default config and passes it with `--config-path` for the command invocation.
- The temporary config file is not written into project root and is cleaned up after execution.
- When the temporary default is used, TSPack prints one concise status line to stderr before Biome starts: `Using TSPack default Biome config: tabs, 100 columns, double quotes, organized imports, recommended lint rules. Add biome.json to customize.`
- Add `biome.json` or `biome.jsonc` to customize or override these defaults.

TSPack's default Biome config is intentionally explicit:

- formatter enabled;
- tab indentation;
- `lineWidth` 100;
- JavaScript/TypeScript double quotes;
- trailing commas `all`;
- semicolons `always`;
- arrow parentheses `always`;
- bracket spacing enabled;
- `organizeImports` enabled;
- linter enabled with recommended rules;
- `lint/style/useImportType` set to error;
- `lint/correctness/noUnusedVariables` set to warning;
- `lint/correctness/noUnusedImports` set to warning.

## Mutation and exit behavior

- `tspack format` writes formatting by invoking Biome format with `--write`.
  - It exits `0` when formatting succeeds.
  - It emits `TSPACK_FORMAT_WRITE_FAILED` and exits nonzero if Biome cannot apply formatting successfully.
- `tspack format --check` is a read-only CI gate and uses Biome's non-write format mode. TSPack does not pass a Biome `--check` flag for `format`.
  - It exits `0` when files are already formatted.
  - It emits `TSPACK_FORMAT_CHECK_FAILED` and exits nonzero when files would change.
  - Run `tspack format` to apply formatting.
- `tspack lint` is read-only by default; no separate `--check` flag is needed because lint without `--fix` is the check.
  - It exits `0` when there are no lint violations.
  - It emits `TSPACK_LINT_CHECK_FAILED` and exits nonzero when Biome reports lint violations.
  - Run `tspack lint --fix` to apply safe fixes where possible.
- `tspack lint --fix` may modify files by invoking Biome lint with `--write` for safe fixes.
  - It exits `0` when the fix attempt leaves no violations.
  - It emits `TSPACK_LINT_FIX_INCOMPLETE` and exits nonzero when violations remain after the fix attempt.
  - Unsafe fixes are not applied by default.
- `tspack lint --fix --unsafe` forwards Biome unsafe fixes by invoking Biome lint with `--write --unsafe`.
  - Unsafe fixes may change runtime semantics or require review. Use this only when you intend to accept Biome's unsafe transformations.
  - Review the resulting diff after the command completes.
  - If violations remain, TSPack still emits `TSPACK_LINT_FIX_INCOMPLETE`; the diagnostic notes that unsafe fixes were enabled for the run.
- `--unsafe` requires `--fix`; `tspack lint --unsafe` and `tspack lint src --unsafe` are invalid.
- `tspack format` and `tspack format --check` do not support `--unsafe`; format has no unsafe behavior in TSPack.

These commands do not install packages or run package-manager scripts.

## Output behavior

TSPack streams Biome stdout and stderr directly to the user. When a TSPack diagnostic is needed, it is printed through the existing diagnostic stream after Biome's output so Biome's actionable file diagnostics remain visible.

## Relationship to `tspack check`

`check` remains architecture/manifest/lock/type/boundary validation by default and does not include linting or formatting unless requested.

`tspack check --format` runs normal project validation plus a read-only Biome format check over `.` relative to the selected root. It uses the same backend resolution and config behavior as `tspack format --check`, including the temporary default config and stderr status line when no project `biome.json` or `biome.jsonc` exists. It does not write files and does not pass Biome `--write` or Biome `--check`. Run `tspack format` to apply formatting changes.

With `tspack check --format --json`, Biome stdout/stderr is captured instead of streamed so stdout remains parseable JSON. Format failures are included in the normal check JSON diagnostics as `TSPACK_FORMAT_CHECK_FAILED`.

## Sync compatibility expectation

When Biome is declared as a direct tool dependency and materialized by `tspack sync`, TSPack generates `node_modules/.bin/biome` as part of strict compatibility materialization and preserves the executable package binary at `node_modules/@biomejs/biome/bin/biome` on POSIX. `tspack format`/`tspack lint` can resolve either local backend without npm script execution.

## Related docs

- `docs/claude-fooding-phase8.md` records the Phase 8 remediation closeout and release-gate expectations.
