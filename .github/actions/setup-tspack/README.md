# setup-tspack

`setup-tspack` installs a TSPack release binary in GitHub Actions and adds it to `PATH`.

Use the first-party action from this repository with the subdirectory path:

```yaml
steps:
  - uses: yuechen-li-dev/tspack/.github/actions/setup-tspack@v0.1.9-m80b1.1
    with:
      version: v0.1.9-m80b1.1

  - run: tspack sync --root .
  - run: tspack check --root .
  - run: tspack test --root .
  - run: tspack pack --verify --package <pkg>
```

GitHub Actions cannot normally resolve `uses: yuechen-li-dev/tspack/setup-tspack@...` unless `setup-tspack` is split into a separate repository. A separate setup action repository may be considered later, but this milestone keeps the action in the TSPack repository and pins its source to the same prerelease tag as the installed binary.

## Inputs

| Input | Default | Description |
| --- | --- | --- |
| `version` | `latest` | TSPack version to install. Use `latest` or a release tag such as `v0.1.8`. |
| `repo` | `yuechen-li-dev/tspack` | Repository to download TSPack releases from. |
| `github-token` | unset | Optional GitHub token for API requests, useful for rate limits or private forks. The action also reads `GITHUB_TOKEN` when this input is not provided. |
| `install-dir` | runner temp `tspack-bin` directory | Directory where the `tspack` binary is installed. |
| `check` | `true` | Run the installed binary with `--help` after installation. |
| `version-file` | `.tspack-version` | Project-relative file containing the minimum compatible TSPack release. Missing files are allowed. |

## Outputs

| Output | Description |
| --- | --- |
| `version` | Installed release tag. |
| `path` | Full path to the installed `tspack` or `tspack.exe` binary. |

## Pinned versions

```yaml
steps:
  - uses: yuechen-li-dev/tspack/.github/actions/setup-tspack@v0.1.9-m80b1.1
    with:
      version: v0.1.9-m80b1.1
```

## Supported runners

The action downloads the GitHub Release artifacts produced by the release matrix:

- Ubuntu/Linux x64 -> `tspack-linux-amd64.tar.gz`
- Ubuntu/Linux arm64 -> `tspack-linux-arm64.tar.gz`
- macOS x64 -> `tspack-darwin-amd64.tar.gz`
- macOS arm64 -> `tspack-darwin-arm64.tar.gz`
- Windows x64 -> `tspack-windows-amd64.zip`

Windows arm64 and other OS/architecture combinations fail with a clear unsupported-platform error.

## Integrity and installation behavior

The action downloads `checksums.txt` from the same GitHub Release as the archive, verifies the archive SHA256 entry before extraction, copies the `tspack` binary into the install directory, and appends that directory to `GITHUB_PATH`.

After resolving `version`, the action reads `.tspack-version` from `GITHUB_WORKSPACE` when present. If the requested or latest release is older than the declared minimum, setup fails with `TSPACK_VERSION_TOO_OLD` before downloading an incompatible binary. The workflow's `uses:` reference must itself point to an action release that supports this check; an older action implementation cannot enforce a contract added later.

The action does not build TSPack from source, does not install npm packages, does not commit or require `node_modules`, does not use `get.tspack.dev`, and does not implement package-manager distribution channels.

## Release smoke

The action is covered by local Node tests in this repository. Before each release is published, keep the action covered by those tests and verify the released artifacts and `checksums.txt` through the release workflow and release-prep smoke path.

## Recommended CI shape

When `manifest.tsx` and committed `ts-lock.toml` are present, a fresh runner should usually start with:

```yaml
- run: tspack sync --root .
- run: tspack check --root .
```

Use `tspack update` in CI only when the workflow intentionally wants to change dependency resolution or refresh the committed lockfile.
