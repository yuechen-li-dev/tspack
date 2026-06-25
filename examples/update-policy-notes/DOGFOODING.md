# Update Policy Dogfood

## Purpose

This fixture dogfoods declared rolling release policy as the TSPack-native alternative to external update-bot churn. It exercises report-only update intent, lifecycle security gates, and no-mutation guarantees on a realistic TypeScript workspace.

## Fixture

`examples/update-policy-notes` contains three packages:

| Package | Role | Notable dependencies |
| --- | --- | --- |
| `@tspack-examples/update-policy-app` | app | `react`, `react-dom`, workspace utils, `typescript`, `vite`, `esbuild` |
| `@tspack-examples/update-policy-lib` | library | `react`, `react-dom`, workspace utils, `typescript`, `vite` |
| `@tspack-examples/update-policy-utils` | shared library | `typescript`, `@biomejs/biome`, `rollup` |

The manifest declares root-level `<UpdatePolicy />` rows for rolling tool updates, manual React runtime updates, and pinned React DOM peer policy. It also declares `<Security />` with:

- an exact `acknowledgedCapabilities` row for the synthetic `@biomejs/biome` candidate `postinstall` lifecycle capability;
- a `maintainer-publish` `acknowledgedLifecycleCategories` row for `prepare` / `prepublishOnly` scripts;
- a behavior fixture reference that is validated as evidence metadata but not executed by policy planning or doctor security.

The lockfile records deterministic current versions. `fake-registry.json` documents the offline metadata shape used by the dogfood tests. Automated tests attach an offline fake registry that supplies wanted/latest candidate versions and lifecycle script metadata, so the dogfood path does not depend on the public npm registry.

## Command matrix

The direct `go run ./cmd/tspack ... --root examples/update-policy-notes` commands are useful for CLI shape checks, but registry-backed outdated/update-policy commands require the offline fake registry used by tests for deterministic results.

| Command | Expected result |
| --- | --- |
| `tspack outdated --root examples/update-policy-notes` | grouped shared tool declarations, policy status visible, workspace dependency not applicable |
| `tspack outdated --root examples/update-policy-notes --json` | grouped JSON entries with policy fields and compatibility `dependencies` |
| `tspack outdated --root examples/update-policy-notes --per-package` | declaration-level output restored |
| `tspack update --policy --dry-run --root examples/update-policy-notes` | dry-run policy plan with Ready, Blocked by security, Blocked by policy, Not applicable, no lockfile writes |
| `tspack update --policy --dry-run --json --root examples/update-policy-notes` | stable `dryRun` and `policyPlan` JSON with boolean `wouldUpdate` / `wouldApply` and non-null summary counts |
| `tspack check --root examples/update-policy-notes` | lifecycle summaries preserve normal check behavior and do not execute scripts |
| `tspack check --root examples/update-policy-notes --show-lifecycle` | lifecycle details visible, including acknowledgment status |
| `tspack doctor security --root examples/update-policy-notes` | exact and category acknowledgments visible, category matches counted |
| `tspack doctor security --root examples/update-policy-notes --json` | machine-readable security audit with lifecycle categories and acknowledgment kinds |
| `tspack update --policy --root examples/update-policy-notes` | fails because policy update requires `--dry-run` |
| `tspack update typescript --policy --dry-run --root examples/update-policy-notes` | fails because targeted policy planning remains unsupported |

## Results

- Outdated grouping keeps shared `typescript` tool declarations readable instead of row-spamming every package declaration.
- Policy status covers ready rolling updates, major updates outside a minor policy, manual/pinned runtime policy, and non-registry workspace dependencies.
- Policy dry-run separates version-policy allowance from security readiness: `typescript`, exact-acknowledged `@biomejs/biome`, and category-acknowledged `rollup` are ready; unacknowledged consumer-install `esbuild` is security-blocked; `vite`, `react`, and `react-dom` are policy-blocked.
- JSON policy plans keep stable arrays for `allowed`, `blocked`, `unclassified`, `notApplicable`, and `noop`, plus candidate `effectiveAction`, `securityGateStatus`, and `securityGateReasons` fields.
- Check and doctor security continue to report lifecycle capabilities and acknowledgments without executing lifecycle scripts or behavior fixtures.
- No-mutation assertions hash/read the lockfile before and after outdated and policy-planning paths; store and materialization directories remain absent in the focused dogfood test.

## Current limitations

- `update --policy` is dry-run only.
- Targeted policy planning is unsupported.
- Security gates use TSPack's lifecycle capability and acknowledgment model only.
- There is no external vulnerability database integration.
- There is no React single-version/coherence policy yet.
- There is no dependency unification policy yet.
- There is no policy-driven mutation yet.

## Verdict

Outcome A / LGTM for M50d: the fixture and tests demonstrate the M49/M50 update-policy arc as a readable, deterministic, read-only workflow with security-gated readiness and dry-run-only guardrails intact.
