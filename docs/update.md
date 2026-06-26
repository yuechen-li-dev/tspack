# Update

`tspack update --policy --dry-run` is a read-only policy planning command. It loads the current manifest, lockfile, registry metadata, declared `<UpdatePolicy />`, and TSPack security policy, then reports which grouped dependency candidates are ready, need security review, are blocked by security, are blocked by version policy, are unclassified, or are not applicable.

This command does not mutate the lockfile, populate the store, materialize dependencies, or run lifecycle scripts. Lifecycle execution remains blocked regardless of whether a candidate passes the policy-plan security gate.

## Security-gated policy plans

Policy planning first evaluates version intent from `<UpdatePolicy />`. It then evaluates allowed candidates against the current TSPack lifecycle capability and acknowledgment model:

- `passed`: the candidate has no lifecycle capability concerns in available registry metadata, or every lifecycle capability is covered by an exact capability acknowledgment or matching lifecycle-category acknowledgment.
- `review_required`: the candidate carries an unacknowledged maintainer-publish or other lifecycle script. The future rolling update is not ready until a human reviews the script or policy changes.
- `blocked`: the candidate carries an unacknowledged consumer-install lifecycle script such as `preinstall`, `install`, or `postinstall`, or another non-passing high-risk lifecycle state. This blocks readiness for policy-driven rolling updates; it does not mean TSPack would execute the script.
- `not_applicable`: the candidate is not a registry package update candidate, or no package change exists.

Human output starts with `Policy update plan (dry run)`, includes `Ready`, `Needs review`, `Blocked by security`, `Blocked by policy`, `Unclassified`, and `Not applicable` sections as applicable, and ends with summary counts plus `security gates: evaluated`, `lifecycle execution remains blocked`, and `lockfile written: no`.

JSON output from `tspack update --policy --dry-run --json` contains the normalized `dryRun` object with `changed: false` because no lockfile diff is generated or applied. Policy intent is represented under `policyPlan`. M50c keeps `policyPlan.wouldUpdate` as the version-policy signal and adds `policyPlan.wouldApply` for candidates that are both allowed by version policy and `passed` by security gates. The summary includes `ready`, `securityBlocked`, and `reviewRequired` counts, and each candidate includes `securityGateStatus`, `securityGateReasons`, `securityGateDiagnostics`, and `effectiveAction`.

`effectiveAction` is `update` for allowed-and-passed candidates, `review` for allowed candidates requiring review, `blocked` for allowed candidates blocked by security, and `skip` for policy-blocked, unclassified, no-op, and not-applicable candidates.

`--policy` without `--dry-run` remains rejected. Targeted policy planning remains rejected. Normal `tspack update` behavior is unchanged.

## Store population throughput

Normal `tspack update` separates deterministic resolution from artifact population. Resolution decides the package graph and lockfile content first; missing store artifacts are then fetched, extracted, hashed, and committed with bounded parallel workers; final diagnostics and lockfile writes remain deterministic.

Set `TSPACK_STORE_JOBS=1` to force sequential store population. Set `TSPACK_STORE_JOBS=N` with a positive integer to tune local cold-update throughput. Invalid values fail clearly. `tspack update --dry-run` and `tspack update --policy --dry-run` remain read-only: they may resolve metadata, but they do not populate `.tspack/store`, write `ts-lock.toml`, materialize `node_modules`, or run lifecycle scripts.
