
## Policy update plan dry run

`tspack update --policy --dry-run` is the M50b read-only policy planning command. It loads the current manifest, lockfile, registry metadata, and declared `<UpdatePolicy />`, then reports which grouped dependency candidates are allowed by rolling policy, blocked by manual/pinned/outside-level policy, unclassified because no policy row matches, or not applicable because the dependency is workspace/path/non-registry.

This command does not mutate the lockfile, populate the store, materialize dependencies, or run lifecycle scripts. Security gates are explicitly not evaluated yet; JSON reports `securityGatesEvaluated: false` and `securityGateStatus: "not_evaluated"`.

`--policy` currently requires `--dry-run`. `tspack update --policy` fails with a diagnostic explaining that policy-driven mutation is not implemented yet. Targeted policy planning is also deferred in M50b, so `tspack update <query> --policy --dry-run` fails and asks for a workspace policy dry run.

Human output starts with `Policy update plan (dry run)`, prints the allowed/blocked/unclassified/not applicable buckets, and ends with `security gates: not evaluated` and `lockfile written: no`. If no `<UpdatePolicy />` exists, the command still succeeds when metadata resolution succeeds, reports that no update policy is declared, and treats registry candidates as unclassified.

JSON output from `tspack update --policy --dry-run --json` contains the normalized `dryRun` object with `changed: false` because no lockfile diff is generated or applied. Policy intent is represented separately under `policyPlan`; use `policyPlan.wouldUpdate` or `policyPlan.summary.allowed` to detect candidates that policy would allow. The command exits 0 when report generation succeeds, even if allowed updates exist. Metadata, resolution, and manifest-validation errors keep their existing failure behavior.
