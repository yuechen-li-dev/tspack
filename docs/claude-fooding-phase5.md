# Claude-Fooding Phase 5 Remediation

Claude-fooding Phase 5 focused on TSPack's declared RunTargets, `tspack run`, and `tspack doctor run`. The closeout state is **Success**: the original runtime-loop findings have explicit remediation milestones, release-gate smoke coverage expectations, and a documented golden run/debugging flow without adding new behavior in this closeout milestone.

## Original findings

Phase 5 identified the following issues in the RunTarget development loop:

- `system` runtime produced a false negative in `tspack doctor run` even though it is a built-in runtime.
- `tspack run` status/progress output was mixed with child stdout.
- `tspack run --manifest <path>` was missing.
- Package-scoped target selection was missing for workspaces with repeated target names.
- Run target listing was missing.
- Command working-directory ergonomics were unclear for workspace-root versus package-root commands.
- Readiness was limited to HTTP polling.
- `tspack run --env KEY=VALUE` was missing for explicit child environment overlays.
- Doctor text output omitted useful runtime and target details.
- Reserved `bun` and `deno` runtimes produced warning noise instead of being reported as not applicable.

## Remediation summary

| Finding | Milestone | Fix | Status |
|---|---|---|---|
| `system` runtime false negative in doctor | M35a | Treat `system` as built in and available. | Complete |
| Run status mixed with child stdout | M35a | Route TSPack status/progress to stderr while preserving child stdout/stderr passthrough. | Complete |
| Missing `tspack run --manifest <path>` | M35a | Allow run to use an explicit manifest path. | Complete |
| Doctor text output missing details | M35a | Print useful runtime, availability, target, cwd, and readiness details. | Complete |
| `bun`/`deno` warning noise | M35a | Report reserved runtimes as `not_applicable` instead of actionable warning noise. | Complete |
| Run target listing missing | M35b | Add `tspack run --list` and `tspack run --list --json`. | Complete |
| Package-scoped target selection missing | M35b | Add package-qualified target identity and `tspack run --package <package> <target>`. | Complete |
| Ambiguous duplicated target names | M35b | Add package-aware ambiguity diagnostics and remediation hints. | Complete |
| Command cwd ergonomics | M35c | Add RunTarget `cwd?: "workspace" | "package"`; omitted/workspace runs from workspace root and package runs from the declaring package root. | Complete |
| Limited readiness kinds | M35d | Preserve HTTP readiness and add TCP and stdout/stderr substring readiness. | Complete |
| Missing explicit env overlays | M35e | Add repeatable `--env KEY=VALUE` overlays with literal values and keys-only status output. | Complete |
| Inspect-run startup divergence | M35e | Reuse the shared startup path for `tspack inspect --run <target> --env KEY=VALUE`. | Complete |

## Current RunTarget model

- **Package-qualified target identity:** every target has an identity shaped as `<package-name>:<target-name>`.
- **Target selection:**
  - explicit `<target>` selects that target when it is globally unambiguous;
  - `--package <package-name> <target>` scopes selection to one package;
  - omitted target falls back to a package or global `dev` target when exactly one applies;
  - omitted target falls back to the single available target when exactly one exists;
  - ambiguous target names fail with package-qualified diagnostics and hints.
- **Working directory policy:**
  - omitted `cwd` and `cwd: "workspace"` run commands from the workspace root;
  - `cwd: "package"` runs commands from the declaring package root;
  - list, doctor, run status, and inspect-run flows expose or reuse the effective cwd behavior.
- **Readiness:** supported readiness kinds are HTTP, TCP, and stdout/stderr substring matching (`stdout-match`).
- **Environment overlays:**
  - `--env KEY=VALUE` is repeatable;
  - values are literal after the user's shell has parsed the command;
  - TSPack does not perform shell interpolation, dotenv loading, or env-file loading;
  - status output prints env keys only, not values.
- **Stream behavior:**
  - TSPack status/progress is written to stderr;
  - child stdout remains on stdout;
  - child stderr remains on stderr.

## Current golden run/debugging flow

```sh
tspack doctor run
tspack doctor run --json
tspack run --list
tspack run --list --json
tspack run --package <pkg> <target>
tspack run <target> --once
tspack run <target> --env PORT=3001
tspack inspect --run <target> --env PORT=3001
tspack how TSPACK_RUN_TARGET_AMBIGUOUS
tspack how TSPACK_RUN_INVALID_ENV
```

## Explicit non-goals / deferred work

- No `bun` or `deno` runtime backend yet.
- No dotenv or env-file loading.
- No manifest-level env declarations.
- No secret manager semantics.
- No shell interpolation.
- No file readiness yet.
- No package-cwd default flip yet.
- No lifecycle scripts.
- No npm script inference.
