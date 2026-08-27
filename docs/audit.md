# Vulnerability audit

`tspack audit` checks the exact npm package name/version pairs in `ts-lock.toml` against the OSV.dev vulnerability database. For mixed locks it also reports coverage by source. Because OSV does not currently define a JSR ecosystem identifier, JSR packages are explicitly marked `not-checked`; they are never silently omitted as though the whole graph were clean. Audit is read-only: it does not execute lifecycle scripts, inspect mutable `node_modules`, change the manifest or lockfile, or attempt automatic remediation.

```text
tspack audit
tspack audit --audit-level high
tspack audit --json
```

Any advisory fails by default. `--audit-level low|moderate|high|critical` raises the CI failure threshold without hiding lower-severity findings. Advisories with no usable severity are reported as `unknown`; they fail the default `any` threshold but do not meet an explicit severity threshold.

The report includes advisory IDs and aliases, locked versions, known fixed versions, references, and lock graph paths. OSV request, response, or schema failures produce `TSPACK_AUDIT_SERVICE_FAILED` and a nonzero exit. A failed scan is never described as clean.

Use `tspack why <package>` to inspect a finding and `tspack update <dependency> --dry-run` to preview a bounded refresh. TSPack intentionally has no `audit fix`: remediation remains an explicit manifest/update decision.

Set `TSPACK_OSV_API` only when testing against an OSV-compatible endpoint.
