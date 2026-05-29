# tspack pack

`tspack pack` creates deterministic local `.tgz` package artifacts.

## Guarantees
- Runs project checks before packing.
- Does not build.
- Does not publish.
- Does not execute scripts.
- Does not mutate `ts-lock.toml`.
- Does not use `node_modules` as package source of truth.

## Publish policy
Files are included only when matched by `publish.include` (package-root-relative), then removed by `publish.exclude` (exclude wins).

Unsafe paths (absolute paths, `..`, backslashes) are rejected.

If `package.json` is not included by policy, tspack generates a deterministic `package/package.json` in-archive from the manifest IR. The generated file includes `name`, `version`, `exports`, and, when available or applicable, package publish metadata: `license`, `main`, `types`, `peerDependencies`, and `peerDependenciesMeta` for optional npm peers.

Generated `main` is derived only from the root export target (`export: "."`) runtime output and is normalized as an npm package-relative path such as `./dist/index.js`. Generated `types` follows the root export target types output when present. Export entries keep the existing manifest target exports and use normalized package-relative paths.

Generated `peerDependencies` are derived from manifest peer dependencies that use npm sources. Tool dependencies are not emitted as runtime dependencies. Non-npm peer sources fail packing with `TSPACK_PACK_UNPUBLISHABLE_PEER_DEPENDENCY` because npm package metadata requires package names with version ranges.

## Determinism
- Stable archive entry order.
- `package/` path prefix.
- Normalized `/` separators.
- Fixed mtime epoch.
- Stable JSON formatting and deterministic key ordering for generated `package.json`, including sorted `peerDependencies` keys.
- SHA-256 hash format: `sha256:<hex>`.

## Workspace behavior
By default, `tspack pack` packs all workspace packages. Use `--package <name>` to pack one package.

## Non-goals (M12)
- No `tspack publish`.
- No npm auth/registry upload.
- No build/test/dev/add/remove/why behavior.


## Empty package policy
If `publish.include` matches no real files, pack fails with `TSPACK_PACK_EMPTY_PACKAGE` even though generated `package.json` is available.

Symlinked files matched by publish policy are rejected with `TSPACK_PACK_SYMLINK_UNSUPPORTED`.
