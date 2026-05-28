# Claude-Fooding Phase 3 Remediation

Claude-fooding Phase 3 focused on TSPack's boundary/import system. The closeout state is **Success**: the original findings have explicit remediation milestones, release-gate coverage expectations, and a documented debugging flow without adding new behavior in this closeout milestone.

## Original findings

- `.js` -> `.ts`/`.tsx` TypeScript ESM aliasing was missing.
- Workspace/path dependency identity matching was incomplete.
- `from` physical-file semantics were non-obvious.
- `check --json` structured boundary diagnostics needed coverage.
- Boundary rule debugging needed `tspack check --explain <file>`.
- Boundary rules needed graph-reachable `transitiveFrom` matching.
- Runtime dependency boundaries needed positive allowlist support through `allowOnly`.
- Type-level imports needed explicit boundary enforcement through `denyTypeDeps`.

## Remediation summary

| Finding | Milestone | Fix | Status |
|---|---|---|---|
| `.js`/`.jsx` imports did not traverse to TypeScript sources during local ESM alias resolution. | M33a | TypeScript ESM `.js`/`.jsx` import aliases resolve to `.ts`/`.tsx` sources. | Complete |
| Workspace/path dependency matching could confuse declared dependency identity with resolved locations. | M33a | Workspace/path dependencies match exact declared identifiers. | Complete |
| `from` semantics were easy to misread as graph reachability rather than the importing file's physical path. | M33b | Documentation and tests define `from` as physical importing-file semantics. | Complete |
| Structured boundary diagnostics in `check --json` needed regression coverage. | M33b | JSON boundary diagnostic coverage was added. | Complete |
| Developers needed a focused way to inspect boundary rule application for one file. | M33c | `tspack check --explain <file>` supports text and JSON output. | Complete |
| Boundary rules needed to cover files reachable from an entry/seed. | M33d | `transitiveFrom` graph-reachable boundary rules were added. | Complete |
| Runtime boundaries needed positive allowlists, not only explicit deny patterns. | M33e | Runtime `allowOnly` boundary rules were added. | Complete |
| Type-only source imports/re-exports needed explicit deny enforcement. | M33f | Type-level source boundary enforcement with `denyTypeDeps` was added. | Complete |

## Current boundary model

- `from` means the physical importing file. A rule with `from: "src/index.ts"` applies to imports written in that exact file, not automatically to files that `src/index.ts` imports.
- `transitiveFrom` means the graph-reachable closure from a seed file. It applies to matching imports found in the seed and in local files reachable from that seed.
- `denyDeps` is a runtime explicit deny control for external runtime imports.
- `allowOnly` is a runtime positive allowlist control for external runtime imports.
- `denyTypeDeps` is a type-level explicit deny control for type-only source imports and re-exports.
- Tool dependencies cannot be runtime imports.
- `tspack check --explain <file>` helps debug target reachability, rule matching, import classification, and boundary decisions for one source file.

## Current golden boundary debugging flow

```sh
tspack check --json
tspack check --explain src/file.ts
tspack how TSPACK_BOUNDARY_EXPLICIT_DENY
tspack how TSPACK_BOUNDARY_ALLOW_ONLY_VIOLATION
tspack how TSPACK_BOUNDARY_TYPE_EXPLICIT_DENY
```

Use `check --json` first to capture the full structured diagnostic set, `check --explain` to inspect one surprising file, and `how` to read remediation guidance for the specific boundary diagnostic code.

## Explicit non-goals / deferred work

- No full TypeScript symbol graph tracing.
- No generated `.d.ts` public surface enforcement yet.
- No `node_modules`-aware module identity verification yet.
- No automatic fixes.
- No type-level `allowOnly` yet.
- No source resolver changes.
