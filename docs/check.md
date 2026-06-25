# Check command

`tspack check` validates the manifest, lockfile, package graph, security capability metadata, and other project invariants without mutating lockfiles, materialized packages, or source files.

## Noisy warning summaries

Default human output keeps serious diagnostics visible and summarizes noisy informational warning families when there are two or more entries:

- Version conflicts (`TSPACK_LOCK_VERSION_CONFLICT`) are summarized with a count, deterministic package/version examples, and `tspack check --show-conflicts` guidance.
- Lifecycle-script-present warnings (`TSPACK_SECURITY_LIFECYCLE_SCRIPT_PRESENT`) are summarized with a count, deterministic package examples, and the lifecycle posture that execution is blocked by policy.

Use these reveal flags when you need the full human diagnostic blocks:

```sh
tspack check --show-conflicts
tspack check --show-lifecycle
tspack check --show-conflicts --show-lifecycle
```

The reveal flags only affect human rendering. They do not change diagnostic generation, security posture, lifecycle execution behavior, or exit-code semantics.

## JSON output

`tspack check --json` remains full-detail and machine-useful. It includes every individual version-conflict and lifecycle-present diagnostic, including lifecycle script details and pull chains when available. Human summary diagnostics are not a replacement for the structured diagnostics array.

## Security visibility

Lifecycle summary output means packages declare scripts, but TSPack's policy blocks lifecycle execution by default. More serious security diagnostics, such as stale or unused acknowledgments and future error-level security blockers, remain visible in default human output. Use `tspack doctor security` for policy posture.


## Lifecycle category summaries

Default human `tspack check` groups lifecycle-script-present warnings by lifecycle category: consumer install-time (`preinstall`, `install`, `postinstall`), maintainer publish-time (`prepublishOnly`, `prepublish`, `prepare`, `prepack`, `postpack`, `publish`, `postpublish`), and `other` detected lifecycle hooks. Consumer install-time counts are highlighted separately from maintainer-side counts, and maintainer-only summaries note that those hooks do not run during normal consumer install in npm-style workflows. Execution remains blocked by policy for every category.

Use `tspack check --show-lifecycle` to reveal the individual diagnostics, including `lifecycleCategory`, `consumerInstallTime`, script commands, and pull chains. `tspack check --json` is not summary-only; it continues to emit individual lifecycle diagnostics with classification fields.

### Lifecycle category acknowledgment summaries

If lifecycle diagnostics are acknowledged by `Security.acknowledgedLifecycleCategories`, default human `tspack check` summarizes the acknowledged category-policy count and keeps unacknowledged lifecycle scripts visible. `tspack check --show-lifecycle` prints each lifecycle diagnostic with its acknowledgment status, including `acknowledgmentKind: lifecycle-category` and `acknowledgedByCategory` when a category policy matched. `tspack check --json` remains full-detail and continues to include individual lifecycle diagnostics, including category, consumer-install-time, and acknowledgment fields.
