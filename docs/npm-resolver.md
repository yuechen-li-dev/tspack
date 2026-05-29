# NPM Resolver (M7)

M7 resolves `source.kind == "npm"` dependency intents from the workspace graph into lockfile targets, packages, and edges.

## Resolver behavior

- Direct target dependencies create edges from `<package>:target:<target>`.
  - Runtime/type deps use `kind = runtime|type`.
  - Direct peer deps use `kind = peer`.
- Tool dependencies create edges from `<package>:tool` with `kind = tool`.
- Transitive package dependencies create edges from `npm:<parent>@<version>`.
  - For optional dependencies, edge uses `kind = runtime` with `optional = true`.

## Semver

- Uses `Masterminds/semver`.
- Supports exact, caret, tilde, and comparator ranges.
- Chooses highest satisfying version deterministically.

## Integrity and tarball checks

- Fetches tarballs through the registry client.
- Verifies SRI for:
  - `sha512-<base64>`
  - `sha256-<base64>`
- Unsupported integrity algorithms emit `TSPACK_RESOLVE_NPM_UNSUPPORTED_INTEGRITY`.
- Parses `package/package.json` from tarball and validates package `name` and `version` against selected metadata.
- Records a tarball SHA-256 content hash in lockfile package `hash`.

## Security: fetch is not execute

- Resolver may fetch metadata/tarballs and inspect archive contents.
- Resolver never executes lifecycle scripts (`preinstall`, `install`, `postinstall`, `prepare`, etc.).
- Lifecycle scripts are recorded as package capabilities: `kind=lifecycleScript`, `script=<script-name>`, `command=<raw-command>`.

## Current M7 limitations

- Non-npm source kinds are skipped with diagnostics (git/path/workspace handled in later milestones).
- No third-party peer solver; only direct target peer edges are modeled in M7.
- Resolve sync mode remains unsupported in M7.
- No CLI update/sync behavior, no store, and no `node_modules` materialization.

## Testing

- Resolver tests are fully offline and deterministic.
- Fake registry metadata and tarballs are generated in test fixtures.
- No live npm network is used in tests.
