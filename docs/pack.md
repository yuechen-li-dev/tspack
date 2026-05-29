# tspack pack

`tspack pack` creates deterministic local `.tgz` package artifacts.

## Guarantees
- Runs project checks before packing.
- Plans and validates every selected package before writing final `.tgz` artifacts.
- Writes artifacts all-or-nothing for the selected package set: if any selected package has an error, no final artifacts are written.
- Writes archives through temporary files and then renames them into place; write failures report `TSPACK_PACK_WRITE_FAILED` and clean temporary files on a best-effort basis.
- Does not build.
- Does not publish.
- Does not execute scripts.
- Does not mutate `ts-lock.toml`.
- Does not use `node_modules` as package source of truth.

## Publish policy
Files are included only when matched by `publish.include` (package-root-relative), then removed by `publish.exclude` (exclude wins).

Every explicit include pattern is a contract. If a `publish.include` pattern matches nothing, pack fails with `TSPACK_PACK_INCLUDE_MATCHED_NOTHING` and writes no archive. This catches forgotten build outputs such as `dist/**` before a stub archive can be produced. Exclude patterns are filters, so an exclude pattern that matches nothing does not fail packing.

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
By default, `tspack pack` packs all workspace packages. Use `--package <name>` to pack one package. The all-or-nothing guarantee applies to the selected package set: a one-package selection writes that package when valid, while a multi-package selection writes none if any selected package fails.

## Dry run
`tspack pack --dry-run` performs the same validation as a real pack, including include-pattern miss checks, and exits nonzero if pack would fail. A valid dry run prints the planned archive entries but writes no artifacts.

## Empty package policy
If publish policy leaves no real files, pack fails rather than producing an archive containing only generated metadata. A `publish.include` pattern that matches nothing is reported as `TSPACK_PACK_INCLUDE_MATCHED_NOTHING`; other empty-content cases use `TSPACK_PACK_EMPTY_PACKAGE`.

Symlinked files matched by publish policy are rejected with `TSPACK_PACK_SYMLINK_UNSUPPORTED`.

## Deferred escape hatches
There is currently no `--continue-on-error` and no `--allow-empty-patterns`. The default strict behavior protects CI and release workflows from partial artifacts and missing-build/stub archives.

## Non-goals (M12)
- No `tspack publish`.
- No npm auth/registry upload.
- No build/test/dev/add/remove/why behavior.
