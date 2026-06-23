# Claude-Fooding Phase 8a Carryover

## Original findings

Phase 8a exercised `tspack migrate` and `tspack update` against a real-ish TypeScript package. The rerun confirmed that the migration flow was useful: dry-run output, the migration report, TODO taxonomy, and source-scan evidence gave actionable review data without mutating project state. Script classification was appropriately honest, and `tspack migrate --check --write` was the right structural validation gate before creating migration outputs.

The original carryover blockers were:

- generated migrate manifests could reference scoped or dashed dependencies through safe TypeScript aliases without preserving npm package identity;
- scoped npm registry metadata URLs could be double encoded;
- resolver tarball package metadata parsing expected only `package/package.json` and missed non-standard `@types/*` archive roots.

## Fix summary

| Finding | Fix | Milestone | Status |
| --- | --- | --- | --- |
| scoped migrate key emission | explicit key when generated identifier differs | M44a | Fixed |
| scoped metadata URL double encoding | exact-once scoped metadata path | M44b | Fixed |
| non-standard tarball root | flexible single-root package.json detection | M44c | Fixed |

## Rerun results

- Migrated the scoped Phase 8a fixture with `tspack migrate --check --root <fixture>` and `tspack migrate --write --check --root <fixture>` through Go coverage. The generated manifest validates, emits explicit keys for `@biomejs/biome`, `@types/react`, and `react-dom`, preserves optional peer metadata, uses string identity refs for target peers, and canonicalizes `./dist/index.js` / `./dist/index.d.ts` to `dist/index.js` / `dist/index.d.ts`.
- Exercised scoped fake-registry metadata requests for `@biomejs/biome`, `@types/react`, and `@babel/core`. The observed paths are `/@biomejs%2Fbiome`, `/@types%2Freact`, and `/@babel%2Fcore`, with no `%25` double encoding.
- Exercised fake-registry tarballs rooted at `babel__core/package.json`, `estree/package.json`, and the legacy `package/package.json`; all valid single-root cases parse and lock with correct names and versions.
- Exercised the composed scoped metadata plus non-standard tarball-root path for `@types/babel__core`; metadata URL encoding and `babel__core/package.json` parsing compose cleanly.
- Kept legacy happy paths for unscoped packages, `package/package.json`, and custom registry prefixes covered by existing resolver tests.

## Remaining notes

- Generated target dependency and peer references now use string package identity refs in migrate output.
- Generated runtime and types paths omit leading `./` segments.
- TODO comments remain review markers and do not fail migrate validation.
- No package-lock translation, package-manager behavior, npm install behavior, LLM calls, or source semantic inference was added.

## Verdict

Phase 8a carryover LGTM.

Remaining wishlist items are deferred:

- incremental migration report;
- workspace-root multi-package migrate;
- `--from-lockfile` or broader lockfile import, if ever justified.
