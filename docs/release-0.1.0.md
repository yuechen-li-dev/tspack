# TSPack 0.1.0 Release Readiness

This is a practical checklist for tagging and publishing TSPack 0.1.0. It is not the release announcement and does not publish or tag anything by itself.

## Status

- First public early release candidate.
- Self-hosted after bootstrap; a trusted checkout or binary is still required first.
- Not production-stable.
- Manifest/API behavior may still change.

## Release notes

Draft release notes live at [docs/releases/v0.1.0.md](releases/v0.1.0.md).

## Must pass before tagging

```bash
./scripts/self-host-smoke.sh
npm --prefix manifest-frontend run build
npm --prefix manifest-frontend run typecheck:manifest-api
npm --prefix manifest-frontend test
npm --prefix extensions/tspack-vscode test
npm --prefix extensions/tspack-vscode run compile
go test ./...
./scripts/build-release.sh
go run ./cmd/tspack --help
go run ./cmd/tspack --version
./dist/tspack --help
./dist/tspack --version
./scripts/package-release.sh --goos linux --goarch amd64 --version v0.1.0-rc --out-dir dist/release-prep-test
git diff --check
git status --short
```

## Version metadata

`tspack --version` prints the CLI version, commit, and build date. Source builds default to `0.1.0-dev`, `unknown`, and `unknown` unless overridden with Go ldflags.

Release scripts inject metadata with:

```text
-X github.com/tspack/tspack/internal/version.Version=<version>
-X github.com/tspack/tspack/internal/version.Commit=<commit>
-X github.com/tspack/tspack/internal/version.Date=<build-date>
```

## Distribution checks

- `.github/workflows/release.yml` triggers on `v*` tag pushes and supports `workflow_dispatch`.
- The workflow builds self-contained release archives using `scripts/package-release.sh`.
- Expected artifacts:
  - `tspack-linux-amd64.tar.gz`
  - `tspack-linux-arm64.tar.gz`
  - `tspack-darwin-amd64.tar.gz`
  - `tspack-darwin-arm64.tar.gz`
  - `tspack-windows-amd64.zip`
  - `checksums.txt`
- `scripts/install.sh` downloads GitHub Release artifacts and verifies `checksums.txt`.
- `setup-tspack` action implementation exists, but live hosted smoke is pending the first public release.
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

See [docs/roadmap.md](roadmap.md) for the post-0.1.0 roadmap. Deferred items include inspect/browser deep testing, policy-driven mutation, targeted policy planning, React/single-version coherence policy, `check --lint`, per-file format diagnostics, pre-commit hook generation, ecosystem distribution channels, `get.tspack.dev`, setup action hosted smoke, and Visionary / VS Code fork work.

## Tagging procedure

Do not run automatically. When the repository is ready and reviewed:

```bash
git status --short
git tag -a v0.1.0 -m "TSPack v0.1.0"
git push origin v0.1.0
```

Then:

- Watch the release workflow.
- Verify release artifacts and `checksums.txt`.
- Download the Linux artifact.
- Verify its checksum.
- Run `tspack --version` from the downloaded artifact.
- Run `tspack check --help` from the downloaded artifact.
- Run `scripts/install.sh` against `v0.1.0`.
- Run a `setup-tspack` hosted smoke manually after the release exists.

## M53a cold update throughput smoke

Before 0.1.0, verify bounded-parallel store population does not affect deterministic package-manager semantics:

- Compare a cold update fixture with `TSPACK_STORE_JOBS=1` and with the default or `TSPACK_STORE_JOBS=4`; lockfiles must match byte-for-byte and package counts must match.
- Run `go test ./internal/project ./internal/store -run 'Update|Store|Parallel|Jobs|DryRun|Target|Concurrent' -count=1`.
- Optional local measurement command: `TSPACK_STORE_JOBS=1 go test ./internal/project -bench StorePopulation -run '^$'`, then repeat with `TSPACK_STORE_JOBS=8` if a store-population benchmark is available in the working tree. CI must not assert a fixed speedup.
- Self-host smoke and `./scripts/build-release.sh` must still pass after parallel store population.
