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


## Install script

`scripts/install.sh` is the first Unix install path for TSPack release artifacts. It downloads from GitHub Releases, verifies the downloaded archive against `checksums.txt`, extracts the `tspack` binary, and installs it without `sudo` by default.

Current canonical artifact source: GitHub Releases for `yuechen-li-dev/tspack`. The installer is kept in this repository for now; when using a raw GitHub URL later, review the script before running it. Do not treat `get.tspack.dev` as live yet.

Supported Unix platforms map to release artifacts as follows:

- Linux `x86_64` / `amd64` -> `tspack-linux-amd64.tar.gz`
- Linux `aarch64` / `arm64` -> `tspack-linux-arm64.tar.gz`
- macOS `x86_64` / `amd64` -> `tspack-darwin-amd64.tar.gz`
- macOS `aarch64` / `arm64` -> `tspack-darwin-arm64.tar.gz`

Windows is not installed through `scripts/install.sh`. Windows users should manually download `tspack-windows-amd64.zip` from GitHub Releases. A Windows installer or PowerShell script is future work.

By default the script installs to `$HOME/.local/bin/tspack`:

```sh
sh scripts/install.sh
```

Install a specific release tag with:

```sh
TSPACK_VERSION=v0.1.5 sh scripts/install.sh
```

Install to a custom user-writable directory with:

```sh
TSPACK_INSTALL_DIR="$HOME/bin" sh scripts/install.sh
```

If `$HOME/.local/bin` is not on `PATH`, add this to your shell profile manually:

```sh
export PATH="$HOME/.local/bin:$PATH"
```

The installer does not mutate shell profiles, does not require `sudo`, does not skip checksum verification, and does not make hidden network calls beyond the GitHub latest-release API and GitHub Release artifact downloads.


## setup-tspack GitHub Action

The first-party GitHub Action lives in `.github/actions/setup-tspack` and installs TSPack directly from GitHub Releases. Use the subdirectory action path from workflows:

```yaml
steps:
  - uses: yuechen-li-dev/tspack/.github/actions/setup-tspack@v1
    with:
      version: latest

  - run: tspack check --root .
  - run: tspack test --root .
```

The action resolves `latest` through the GitHub Releases API or accepts a pinned tag such as `v0.1.5`, downloads the matching release artifact plus `checksums.txt`, verifies the artifact SHA256, extracts the binary, installs it into a runner-temp bin directory by default, and appends that directory to `GITHUB_PATH`.

Supported action artifacts are Linux `amd64`/`arm64`, macOS `amd64`/`arm64`, and Windows `amd64`. Windows `arm64` and other unsupported combinations fail before download. The action uses the Node 20 GitHub Actions runtime and only Node built-ins; it does not build from source, install npm dependencies, use `get.tspack.dev`, or implement any package-manager distribution channel.

Because the action is stored in this repository, external workflows must use `yuechen-li-dev/tspack/.github/actions/setup-tspack@v1`; `yuechen-li-dev/tspack/setup-tspack@v1` would require a future separate `setup-tspack` repository.

## Future distribution TODOs

- `get.tspack.dev` installer endpoint. This is a future canonical URL and is not live yet.
- GitHub Releases matrix.
- Separate `setup-tspack` GitHub Action repository, if the subdirectory action path becomes undesirable.
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
./scripts/package-release.sh --goos linux --goarch amd64 --version v0.1.5
```

`scripts/install.sh` downloads these GitHub Release artifacts on Unix platforms. `get.tspack.dev` remains a future TODO and is not a live distribution surface yet.

`tspack --version` prints version, commit, and build date metadata. `scripts/build-release.sh` and `scripts/package-release.sh` inject this metadata with Go ldflags; release workflow builds pass the release tag as the version and `GITHUB_SHA` as the commit.
