# Typed workflows

TSPack workflows declare project-level execution intent in `manifest.tsx`. The
manifest produces provider-neutral Workflow IR; local execution and provider
export consume the same deterministic plan. YAML is generated provider output,
not the semantic source.

```tsx
<Workflows
  rows={[
    Workflow("CI", {
      triggers: [Push({ branches: ["main"] }), PullRequest()],
      jobs: [
        Job("validate", {
          runsOn: CurrentHost(),
          steps: [Sync(), Check(), Test(), Build(), Audit()],
        }),
        Job("package", {
          needs: ["validate"],
          runsOn: CurrentHost(),
          steps: [Pack()],
        }),
      ],
    }),
  ]}
/>
```

`Sync`, `Check`, `Test`, `Build`, `Pack`, and `Audit` are semantic operations.
They call typed lifecycle entrypoints directly and never spawn `tspack`, npm
scripts, or provider-specific commands. The CLI and workflow therefore share
selection, diagnostics, cancellation context, and structured result contracts.

The natural defaults use project truth:

```tsx
Test({ filter: "unit" })
Build({ packages: ["app"], targets: ["browser"] })
Audit({ auditLevel: "high", requireCoverage: true })
```

Build supports declared package and compiler-target identities. Test reuses the
existing harness filter; package targeting remains rejected because the current
test application does not define package-scoped discovery. Audit reuses the
lockfile source model, OSV npm coverage, unsupported-source distinctions, and
the existing severity threshold vocabulary.

## Commands

```text
tspack workflow list
tspack workflow inspect CI
tspack workflow inspect CI --json
tspack workflow run CI --jobs 4
tspack workflow run CI --json
tspack workflow export github CI
tspack workflow export github CI --check
```

Inspection shows job dependencies, platforms, argv, working-directory intent,
environment names, secret identities, and declared capabilities. JSON inspect
emits the normalized execution plan without terminal text.

## Effects and secrets

Use `Process` for an executable plus an argument vector. It never invokes a
shell implicitly. `ShellScript` is the explicit escape hatch when shell
interpretation is actually required.

```tsx
Process("verify", {
  command: ["some-tool", "--verify"],
  cwd: "workspace",
  env: [WorkflowEnv("TOKEN", Secret("CI_TOKEN"))],
  capabilities: ["process", "workspaceRead", "environment", "secrets"],
})
```

Environment values are either `Plain(...)` values or `Secret(...)` references.
The IR contains only the secret identity. Local execution resolves it from the
explicit process environment, reports a missing secret separately, passes the
value through the child environment, and redacts known secret material from
captured output. Provider output contains provider secret references, never
resolved values.

Process working directories are either `workspace` or `package:<identity>`.
Package roots are resolved from manifest truth and checked to remain inside the
workspace.

## Planning and failure

Matrices expand into a typed Cartesian product before scheduling. Stable axis
ordering produces collision-safe identities such as
`test[mode=7:"debug",os=7:"linux"]`. A job runs
after all expanded prerequisite instances succeed. Independent jobs may run in
parallel up to `--jobs`; failed or cancelled prerequisites block dependents.
A failed step fails its job and skips later steps.

Ctrl+C cancels active process steps and prevents new jobs from starting.
`timeoutSeconds` is available on steps and cancels the application context used
by native operations. Test and Audit own context-aware child/network work. The
legacy compiler adapters use the same cancellation context and owned process-tree
cleanup as other lifecycle processes; moving their implementation files below
CLI remains follow-up architecture work.
Conditions, retries, outputs, provider
artifacts, and user-authored cache keys are deliberately deferred.

## GitHub Actions

M74 uses thin-runner mode. Export lowers portable triggers, checks out the
repository, uses the release-backed first-party setup action, and runs:

```text
tspack workflow run CI --ci-provider github
```

This keeps one semantic executor for local and GitHub use. The generated path
is `.github/workflows/tspack-<workflow>.yml`, begins with a generated-file
marker, has deterministic ordering, and is validated with a YAML parser.
`--check` detects drift without overwriting. Existing hand-written provider
files are never adopted or deleted automatically.

GitHub expression syntax is created only inside the adapter. The Workflow IR
never contains `${{ ... }}`. Thin-runner export currently accepts Linux and
`CurrentHost` jobs; incompatible semantic platforms produce
`TSPACK_WORKFLOW_PROVIDER_UNSUPPORTED` instead of silent degradation.

The migration from provider programming is intentionally direct:

```yaml
- run: npm ci
- run: npm test
- run: npm run build
```

becomes semantic project intent: `Sync()`, `Test()`, `Build()`, and `Audit()`.
Checkout,
runtime setup, and provider secret spelling remain backend mechanics.
