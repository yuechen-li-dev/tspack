# Migration Closeout

## Original motivation

Existing JavaScript projects usually accrete package metadata, npm scripts, bundler configuration, TypeScript configuration, framework conventions, generated output paths, and source imports over time. That `package.json`/script/config soup is useful evidence, but it is not a complete model of the project.

TSPack is intentionally stricter: a manifest has explicit packages, dependency intent, targets, publish policy, boundaries, and reviewable security posture. Existing npm-style projects therefore need an adoption bridge that can do the mechanical migration work without pretending that package metadata fully encodes architectural intent.

The migration command is that bridge. `tspack migrate` is expected to generate a high-quality draft manifest and an evidence report for human or LLM review. It should perform the boring mechanical 80-90%, preserve uncertainty with stable `MIGRATION_TODO_*` comments, and stop short of unsafe semantic claims.

## Migration thesis

- `package.json` provides mechanical declared intent: package metadata, declared dependencies, package entry fields, publish file hints, and scripts.
- `package-lock.json` provides resolved evidence, not TSPack truth. It can reveal resolved direct versions, package capabilities, duplicate versions, peers, and platform/native packages, but it is not translated into `ts-lock.toml`.
- Source scanning provides observed usage evidence, not architecture truth. Imports can identify runtime/type-only/missing/dev-runtime/builtin patterns, but they do not prove target boundaries or dependency classification.
- Scripts provide RunTarget suggestions, not automatic command migration. Runtime-like scripts can be reviewed as possible `RunTargets`; build, test, lint, format, lifecycle, and maintenance scripts remain report-only evidence.
- Validation proves structural compatibility, not semantic completion. `--check` shows the generated draft is accepted by the manifest frontend and Go IR validator; it does not prove dependency resolution, target correctness, publish completeness, or script migration.
- TODOs are first-class review markers. Stable `MIGRATION_TODO_*` comments are intentional handoff points, not failures by themselves.

## Remediation / implementation summary

| Need / finding | Milestone | Fix | Status |
|---|---|---|---|
| Existing npm projects needed a safe first draft without mutating real manifests. | M41a | Added `tspack migrate` with dry-run default, `--write` outputs, collision protection, mechanical package metadata/dependency/target/publish mapping, script reporting, and stable TODO comments. | Complete |
| Lockfiles had useful evidence but should not become TSPack lock truth. | M41b | Added report-only npm package-lock v2/v3 evidence for direct resolved versions, lifecycle scripts, bins, peers, platform/native packages, duplicate versions, `@types` evidence, and approximate fanout; invalid implicit locks warn, explicit invalid/missing locks fail. | Complete |
| Source imports could catch review risks without semantic analysis or mutation. | M41c | Added read-only source import scan evidence for runtime, type-only, missing, dev-runtime, and builtin imports; added conservative manifest/report TODO comments. | Complete |
| Scripts needed reviewable classification without execution or broad conversion. | M41d | Added package.json script classification and report-only RunTarget suggestions for likely runtime/dev-server scripts; build/test/lint/format/lifecycle scripts are not converted. | Complete |
| Drafts needed a release-gate validation loop before writes. | M41e | Added `tspack migrate --check`, validating the generated draft through the manifest frontend and Go IR validator; dry-run check writes nothing, `--write --check` validates before writing, validation failures block writes, and TODOs are counted but not failures. | Complete |
| Migration track needed closeout documentation and release-gate coverage. | M41f | Documented the closeout model, TODO taxonomy, golden workflow, non-goals, and migration smoke checklist. | Complete |

## Current migrate model

`tspack migrate` is an onboarding/draft generator for package.json projects:

- Dry-run by default: `tspack migrate` previews inputs, output paths, summary counts, and TODOs without writing files.
- `--write` writes `manifest.migrated.tsx` and `tspack-migration.md`; it does not overwrite `manifest.tsx` by default and does not mutate `package.json`.
- `--force` permits overwriting migration output files after the user opts in; output collision protection otherwise fails before partial writes.
- `--check` validates the generated draft through the manifest frontend and Go IR validator. Dry-run check uses temporary output and writes nothing. `--write --check` validates before writing; failed validation blocks writes.
- `manifest.migrated.tsx` is a readable draft using the current manifest authoring API and stable `MIGRATION_TODO_*` comments.
- `tspack-migration.md` is the evidence report: inputs, mechanical mappings, package-lock evidence, source scan evidence, script classification, validation status, grouped TODOs, security notes, and next steps.
- Package.json mapping covers metadata, package kind hints, dependencies, peer dependencies, optional peers, optional dependencies, dev tools, targets from entry/export fields, and publish include hints.
- Package-lock evidence is report-only and includes direct resolved versions, lifecycle/bin/peer/platform/native/duplicate/`@types`/fanout evidence when available.
- Source scan evidence is read-only and reports runtime imports, type-only imports, missing declarations, dev-runtime mismatches, builtins, warnings, and truncation status.
- Script classification emits report-only RunTarget suggestions for likely runtime processes and review notes for shell/env usage; scripts are never executed.
- The posture is non-execution and non-mutation: migrate does not install packages, run package scripts, mutate source/package files, generate `ts-lock.toml`, call LLM APIs, or run update/sync/package-manager flows.

## TODO taxonomy

| TODO tag | Meaning |
|---|---|
| `MIGRATION_TODO_TARGETS` | Verify target names, source entries, runtime outputs, declaration outputs, and export mapping. |
| `MIGRATION_TODO_DEP_CLASSIFICATION` | Verify dependency kind, directness, optional/runtime/tool semantics, and any target scoping. |
| `MIGRATION_TODO_RUN_TARGETS` | Review scripts and report-only RunTarget suggestions manually; migrate does not insert active RunTargets. |
| `MIGRATION_TODO_PUBLISH` | Verify publish contents and file globs, especially before `tspack pack --dry-run`. |
| `MIGRATION_TODO_BOUNDARIES` | Replace conservative boundary defaults with project-specific architecture policy where needed. |
| `MIGRATION_TODO_TYPES` | Verify declaration output, type-only dependency intent, and public type leakage policy. |
| `MIGRATION_TODO_SECURITY` | Review lifecycle scripts, package bins, native/platform capabilities, and executable package evidence. |

## Current golden migration workflow

```sh
tspack migrate
tspack migrate --write
tspack migrate --check
tspack migrate --write --check
# review manifest.migrated.tsx
# review tspack-migration.md
# resolve MIGRATION_TODO_* comments
tspack check --manifest manifest.migrated.tsx
tspack update --manifest manifest.migrated.tsx
tspack pack --dry-run --manifest manifest.migrated.tsx
```

`migrate --check` is structural validation only. It proves that the generated draft can pass the current manifest frontend and Go IR validator; it does not resolve packages, infer architecture, execute scripts, verify publish output, or prove semantic completion.

A user or LLM reviewer should resolve the `MIGRATION_TODO_*` comments and use the evidence report before treating `manifest.migrated.tsx` as authoritative.

## Explicit non-goals / deferred work

- No package-lock to `ts-lock.toml` translation.
- No dependency resolution during migrate.
- No package install.
- No source or package.json mutation.
- No script execution.
- No automatic RunTarget insertion.
- No exact target-boundary inference.
- No semantic TypeScript checking.
- No tsconfig path alias resolution.
- No bundler/framework analysis.
- No LLM API calls.
- No automatic TODO resolution.

## Future migration ladder

Potential future steps, if justified by user demand and evidence quality:

- Workspace/monorepo package discovery.
- pnpm/yarn lock evidence.
- tsconfig/path alias evidence.
- Framework-specific evidence modules.
- Source scan target-cluster hints.
- Migration reviewer prompts/templates.
- Optional package-lock to `ts-lock.toml` import, only if it can be justified without weakening TSPack lock semantics.
