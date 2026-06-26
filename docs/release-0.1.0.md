# TSPack 0.1.0 Release Readiness

This is a practical checklist for tagging and publishing TSPack 0.1.0. It is not the release announcement and does not publish or tag anything by itself.

## Must pass before tagging

```bash
./scripts/self-host-smoke.sh
cd manifest-frontend && npm run build
cd manifest-frontend && npm run typecheck:manifest-api
cd manifest-frontend && npm test
cd extensions/tspack-vscode && npm test
cd extensions/tspack-vscode && npm run compile
go test ./...
./scripts/build-release.sh
git diff --check
git status --short
```

## Distribution checks

- Release workflow exists.
- Package-release script exists.
- `scripts/install.sh` exists.
- `setup-tspack` action exists, but live hosted smoke is pending the first public release.
- `checksums.txt` is produced by the release workflow.
- `get.tspack.dev` is not live; it remains future work.

## Self-host checks

- Root `manifest.tsx` parses.
- Root `ts-lock.toml` is committed intentionally.
- `./scripts/self-host-smoke.sh` passes.
- The self-host smoke reports no tracked mutation.
- `tspack check --format --root .` passes.
- `tspack doctor security --root . --json` works.
- `tspack update --policy --dry-run --root . --json` works.

## Known deferred items

- Phase 11 inspect/browser deep testing.
- Policy-driven mutation.
- Targeted policy planning.
- React/single-version coherence policy.
- Homebrew/mise/npm bootstrapper.
- `check --lint`.
- Per-file format diagnostics.
- Pre-commit hook generation.
- Visionary / VS Code fork.
- `setup-tspack` hosted smoke after the first release exists.
- `get.tspack.dev`.

## Tagging notes

- Use the `v0.1.0` tag.
- The release workflow uploads artifacts.
- Verify uploaded artifacts and `checksums.txt`.
- Run `scripts/install.sh` against the release artifacts.
- Run a `setup-tspack` action smoke after the release exists.
