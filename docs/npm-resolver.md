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

## Registry metadata URLs

- Package metadata is requested by appending one encoded package-name segment to the registry base URL.
- Unscoped packages use paths such as `/react` and `/left-pad`.
- Scoped packages keep the scope/name slash inside the encoded package-name segment, so `@types/react`, `@biomejs/biome`, and `@babel/core` are requested at paths such as `/@types%2Freact`, `/@biomejs%2Fbiome`, and `/@babel%2Fcore`.
- Scoped metadata paths must not be double encoded; `%25` in place of the package-name escape is a resolver bug.
- Custom registry base paths are supported, so a base such as `https://registry.example.test/npm/` requests `@types/react` at `https://registry.example.test/npm/@types%2Freact`.

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
