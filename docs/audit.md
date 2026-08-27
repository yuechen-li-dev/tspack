# Vulnerability audit

Registry source policy and vulnerability findings are separate signals. The
default permits unsupported coverage (currently JSR) while reporting it
honestly. `requireAuditCoverage={true}` makes `tspack check` reject a lock whose
source lacks an audit ecosystem mapping.

`tspack audit` checks the exact npm package name/version pairs in `ts-lock.toml` against the OSV.dev vulnerability database. For mixed locks it reports the whole locked package count, the checked npm count, per-source coverage, and whether coverage is complete. Audit is read-only: it does not execute lifecycle scripts, inspect mutable `node_modules`, change the manifest or lockfile, or attempt automatic remediation.

Coverage statuses are semantic rather than optimistic:

- `checked` means the exact source/name/version was queried in a configured vulnerability ecosystem;
- `unsupported-ecosystem` means the source is known but OSV has no usable ecosystem mapping, currently JSR;
- `coverage-unknown` means no vulnerability mapping is configured for that package source;
- `not-checked` is reserved for a package set that was not checked for another explicit reason.

JSR's `@jsr/scope__package` compatibility spelling is a Node package-tree
detail, not npm vulnerability identity. It is never sent to OSV as though the
JSR release were an npm release. A mixed clean result therefore says no known
vulnerabilities were found **in checked packages** and explicitly says coverage
is incomplete.

```text
tspack audit
tspack audit --audit-level high
tspack audit --json
```

Any advisory fails by default. `--audit-level low|moderate|high|critical` raises the CI failure threshold without hiding lower-severity findings. Advisories with no usable severity are reported as `unknown`; they fail the default `any` threshold but do not meet an explicit severity threshold.

The report includes advisory IDs and aliases, locked versions, known fixed versions, references, and lock graph paths. OSV request, response, or schema failures produce `TSPACK_AUDIT_SERVICE_FAILED` and a nonzero exit. A failed scan is never described as clean.

JSR's native API can expose linked GitHub repository data, exports, immutable
file checksums, and module metadata. JSR also supports provenance for qualifying
GitHub Actions publications. Those facts can support future provenance
inspection, but neither a repository link nor a provenance page establishes an
OSV ecosystem mapping or npm package/version equivalence. Current JSR
documentation describes signed manifests and npm-compatibility tarball
attestations as future work, so TSPack does not claim verification that the
compatibility tarball corresponds to a provenance statement.

Use a source-qualified query such as `tspack why jsr:@std/path` when investigating a mixed graph, and use `tspack update <dependency> --dry-run` to preview a bounded refresh. An unqualified `why` query reports candidates when the same name exists in several sources. TSPack intentionally has no `audit fix`: remediation remains an explicit manifest/update decision.

Set `TSPACK_OSV_API` only when testing against an OSV-compatible endpoint.
