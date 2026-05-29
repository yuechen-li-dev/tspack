# tspack pack

`tspack pack` creates deterministic local `.tgz` package artifacts.

## Guarantees
- Runs project checks before packing.
- Plans and validates every selected package before writing final `.tgz` artifacts.
- Writes artifacts all-or-nothing for the selected package set: if any selected package has an error, no final artifacts are written.
- With `--verify`, writes temporary archives, verifies every selected artifact, and only then renames archives into final paths.
- Writes archives through temporary files and then renames them into place; write and verification failures clean temporary/final files on a best-effort basis.
- Does not build.
- Does not publish.
- Does not execute scripts.
- Does not mutate `ts-lock.toml`.
- Does not use `node_modules` as package source of truth.

## Publish policy
Files are included only when matched by `publish.include` (package-root-relative), then removed by `publish.exclude` (exclude wins).

Every explicit include pattern is a contract. If a `publish.include` pattern matches nothing, pack fails with `TSPACK_PACK_INCLUDE_MATCHED_NOTHING` and writes no archive. This catches forgotten build outputs such as `dist/**` before a stub archive can be produced. Exclude patterns are filters, so an exclude pattern that matches nothing does not fail packing.

Unsafe paths (absolute paths, `..`, backslashes) are rejected.

TSPack does not auto-include changelogs. If a package root has `CHANGELOG.md` but the final `publish.include` / `publish.exclude` policy omits it, pack emits `TSPACK_PACK_CHANGELOG_NOT_INCLUDED` as a warning and continues. To publish a changelog, declare it explicitly:

```tsx
<Publish include={["dist/**", "README.md", "LICENSE", "CHANGELOG.md"]} />
```

The warning is non-fatal because some packages intentionally omit changelogs. Keeping the file list explicit preserves deterministic, auditable package contents. The same planning warning appears during normal pack, `tspack pack --dry-run`, and `tspack pack --verify`; verification does not require a changelog.

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

## Verification
`tspack pack --verify` creates the same deterministic `.tgz` archive that normal pack would create, but keeps it at a temporary path until structural verification succeeds. Verification inspects the produced tarball as an npm package artifact; it does not reinterpret the tarball as a TSPack source workspace unless the artifact itself contains a manifest as ordinary package content.

Verification checks:

- the archive is readable as gzip/tar, every entry is under `package/`, and entry names do not use absolute paths, parent traversal, or backslashes;
- `package/package.json` exists and parses as JSON;
- package metadata matches the manifest-derived pack plan: `name`, `version`, `license` when declared, root `main`, root `types`, and generated `exports`;
- package path references from `main`, `types`, and string leaves inside `exports` are safe package-relative paths and point at files present in the archive;
- npm peer dependencies and optional peer metadata match the manifest-derived peer dependency metadata;
- the archive contains at least one real payload entry in addition to `package/package.json`.

Verification is intentionally structural only. It does not run `npm install`, fetch registry metadata, publish, execute lifecycle scripts, run package scripts, or execute code from the package. If any selected artifact fails verification, pack exits nonzero and no final artifact from the selected set remains.

## Dry run
`tspack pack --dry-run` performs the same validation as a real pack, including include-pattern miss checks, and exits nonzero if pack would fail. A valid dry run prints the planned archive entries but writes no artifacts. `tspack pack --dry-run --verify` is rejected with `TSPACK_PACK_INVALID_ARGS` because verification requires a produced archive to inspect.

## Empty package policy
If publish policy leaves no real files, pack fails rather than producing an archive containing only generated metadata. A `publish.include` pattern that matches nothing is reported as `TSPACK_PACK_INCLUDE_MATCHED_NOTHING`; other empty-content cases use `TSPACK_PACK_EMPTY_PACKAGE`.

Symlinked files matched by publish policy are rejected with `TSPACK_PACK_SYMLINK_UNSUPPORTED`.

## Deferred escape hatches
There is currently no `--continue-on-error` and no `--allow-empty-patterns`. The default strict behavior protects CI and release workflows from partial artifacts and missing-build/stub archives.

## Non-goals (M12)
- No `tspack publish`.
- No npm auth/registry upload.
- No build/test/dev/add/remove/why behavior.

## Related docs

- See `docs/claude-fooding-phase6.md` for the Phase 6 pack/why remediation closeout.
