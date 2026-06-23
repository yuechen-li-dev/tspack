# Distribution

TSPack release binaries are intended to be self-contained for the JavaScript bridge files that the Go CLI needs at runtime.

## Bridge assets

Development and source builds use filesystem bridge discovery from `manifest-frontend/dist`. Build those files locally with:

```sh
cd manifest-frontend && npm run build
```

Release builds embed the manifest frontend bridge entrypoints with the Go build tag `tspack_embedded_bridges`. The embedded asset file is generated locally or in CI and is not committed to Git.

```sh
./scripts/build-release.sh
```

The release build flow is:

1. Build `manifest-frontend/dist`.
2. Run `go run ./tools/generate-embedded-bridges`.
3. Build `./cmd/tspack` with `-tags tspack_embedded_bridges`.
4. Smoke the binary with `manifest-frontend/dist` temporarily hidden, using trap cleanup so the local frontend build is restored on success, command failure, smoke failure, or handled interrupts.
5. Confirm release smokes do not report missing bridge diagnostics. The inspect smoke targets a local URL far enough to prove the embedded inspect bridge resolves, then accepts only stable later-stage inspect diagnostics such as browser, Playwright, page-load, or invalid-target failures.

Generated files such as `internal/embeddedbridges/generated_assets.go`, copied bridge bundles under `internal/embeddedbridges/assets/`, release binaries under `dist/`, and frontend or extension `dist` trees must stay out of source control.

## Future distribution TODOs

- `get.tspack.dev` installer.
- GitHub Releases matrix.
- `setup-tspack` GitHub Action.
- mise plugin.
- Homebrew tap.
- npm bootstrapper.
- `llms.txt` / agent onboarding.

## GitHub Releases

GitHub Releases are the current canonical source for release archives. The release workflow runs only for `v*` tag pushes and manual dispatches, builds self-contained binaries with embedded bridge assets, and uploads the archives plus `checksums.txt` to the matching GitHub Release.

Initial release archives are:

- `tspack-linux-amd64.tar.gz`
- `tspack-linux-arm64.tar.gz`
- `tspack-darwin-amd64.tar.gz`
- `tspack-darwin-arm64.tar.gz`
- `tspack-windows-amd64.zip`

Each archive contains the `tspack` binary, or `tspack.exe` for Windows, plus `LICENSE` and `README.md` when those files are present in the repository. Unix archives preserve the binary executable bit.

`checksums.txt` is generated with SHA256 entries in this format:

```text
<sha256>  <filename>
```

Signing is not configured yet, so the release workflow does not produce `checksums.txt.sig`.

Build a local host release binary and run the no-dist embedded bridge smoke with:

```sh
./scripts/build-release.sh
```

Build a specific release archive locally with:

```sh
./scripts/package-release.sh --goos linux --goarch amd64 --version v0.1.0
```

Future installers such as `install.sh` and `get.tspack.dev` should download these GitHub Release artifacts. They are not live distribution surfaces yet.

Before the public `v0.1` release, release builds should inject explicit CLI version metadata if the command surface grows a stable `tspack --version` entrypoint.
