# Claude-Fooding Phase 6 Remediation

## Original findings

Phase 6 focused on `tspack pack` and `tspack why`. It found a strong base with deterministic pack output and stable `sha256` reporting, a clear `pack --dry-run` contents preview, useful `why` lock-ID queries, and useful `--package` scoping for both pack and why.

It also identified gaps to close before treating pack/why as release-gate-grade workflows:

- generated `package.json` was missing publish-relevant metadata such as `peerDependencies`, `license`, and `main`;
- pack could partially write artifacts when a multi-package pack had mixed success;
- include patterns that matched nothing were warning-only, which could hide missing build output;
- why bare transitive-name misses did not suggest full lock-ID queries;
- why lock edges repeated globally instead of staying scoped to the matching declaration;
- pack needed a `--verify` mode for structural archive validation;
- why needed deterministic structured output via `--json`;
- why needed reverse root-to-transitive path explanations via `--reverse`;
- changelog convention handling needed to warn without silently changing publish contents.

## Remediation summary

| Finding | Milestone | Fix | Status |
| --- | --- | --- | --- |
| Generated npm package metadata was incomplete. | M36a | Generated `package.json` now includes `license`, `main`, `types`, npm `peerDependencies`, and optional `peerDependenciesMeta`; non-npm peers fail pack with `TSPACK_PACK_UNPUBLISHABLE_PEER_DEPENDENCY`. | Complete |
| Multi-package pack could leave partial final artifacts. | M36b | Pack plans and validates the selected package set before writing and uses temp-write/final-rename semantics for all-or-nothing final artifacts. | Complete |
| Include patterns that matched nothing were warning-only. | M36b | `publish.include` misses are errors by default with `TSPACK_PACK_INCLUDE_MATCHED_NOTHING`. | Complete |
| Dry-run could drift from real pack validation. | M36b | `pack --dry-run` performs pack validation and writes no artifacts. | Complete |
| Bare transitive package-name misses lacked actionable lock-ID guidance. | M36c | Normal why suggests full lock-ID queries for matching locked transitives. | Complete |
| Lock edges repeated globally in why output. | M36c | Declaration lock edges are scoped and deduplicated. | Complete |
| Produced archives lacked a structural verification gate. | M36d | `pack --verify` verifies produced npm artifacts before final success. | Complete |
| Why output needed deterministic structured form. | M36e | `why --json` emits structured explanations and diagnostics with clean JSON stdout. | Complete |
| Why needed inverse dependency-path explanations. | M36f | `why --reverse` reports root-to-query reverse dependency paths. | Complete |
| Changelog convention risk could surprise package consumers. | M36g | Pack warns with `TSPACK_PACK_CHANGELOG_NOT_INCLUDED` when `CHANGELOG.md` exists but the final explicit publish policy omits it. | Complete |

## Current pack model

- Pack generation is deterministic: archive entry ordering, `package/` prefixes, path separators, mtimes, JSON formatting, and reported `sha256:<hex>` values are stable.
- Generated `package/package.json` includes manifest-derived publish metadata when applicable: `license`, `main`, `types`, `exports`, `peerDependencies`, and `peerDependenciesMeta` for optional npm peers.
- Pack is all-or-nothing for the selected package set. A default workspace pack writes no final artifacts if any selected package fails validation or verification.
- Every `publish.include` pattern must match at least one file before exclusions are applied; misses fail with `TSPACK_PACK_INCLUDE_MATCHED_NOTHING`.
- `tspack pack --dry-run` validates the real pack plan, prints planned contents, and writes nothing.
- `tspack pack --verify` structurally checks produced npm artifacts before finalization, including metadata and referenced package paths.
- The explicit publish policy remains authoritative: pack does not infer extra package files beyond the manifest policy and generated metadata.
- `CHANGELOG.md` is not auto-included. If it exists but final policy omits it, pack warns and leaves the package contents unchanged.

## Current why model

- Normal `tspack why <query>` explains declared dependency, target, package, and lock-ID matches without mutating the lockfile.
- Bare transitive package-name misses suggest exact full lock-ID queries when matching lock packages exist.
- Lock edges in explanations are declaration-scoped and deduplicated instead of repeated globally.
- `tspack why --json` emits deterministic structured explanations and diagnostics with JSON-only stdout for handled paths.
- `tspack why --reverse <query>` explains which declared roots pull a lock package in, using root-to-query paths.
- `--package <pkg>` scopes both normal why explanations and reverse why roots to the selected package.

## Current golden pack/why flow

```bash
tspack pack --dry-run
tspack pack --package <pkg> --verify
tspack pack
tspack why react
tspack why react --package <pkg>
tspack why npm:loose-envify@1.4.0
tspack why loose-envify
tspack why react --json
tspack why --reverse loose-envify
tspack how TSPACK_PACK_INCLUDE_MATCHED_NOTHING
tspack how TSPACK_PACK_CHANGELOG_NOT_INCLUDED
tspack how TSPACK_WHY_NOT_FOUND
```

## Explicit non-goals / deferred work

- No npm publish command yet.
- No registry or network verification in `pack --verify`.
- No npm install smoke tests inside `pack --verify`.
- No lifecycle/security capability audit in Phase 6.
- No changelog auto-inclusion.
- No `pack --changelog`.
- No why security audit.
- No dependency convergence policy.

Phase 6 remediation is complete. The closeout state is **Success**: pack and why now have deterministic, structurally verified, explainable release-gate coverage without expanding into publish, install, audit, lifecycle, registry, lockfile schema, manifest DSL, or package-manager resolution changes.
