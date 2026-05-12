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

If `package.json` is not included by policy, tspack generates a deterministic minimal `package/package.json` in-archive.

## Determinism
- Stable archive entry order.
- `package/` path prefix.
- Normalized `/` separators.
- Fixed mtime epoch.
- Stable JSON formatting for generated `package.json`.
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
