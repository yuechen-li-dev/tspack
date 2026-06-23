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
4. Smoke the binary with `manifest-frontend/dist` temporarily hidden.

Generated files such as `internal/embeddedbridges/generated_assets.go`, copied bridge bundles, and frontend `dist` trees must stay out of source control.

## Future distribution TODOs

- `get.tspack.dev` installer.
- GitHub Releases matrix.
- `setup-tspack` GitHub Action.
- mise plugin.
- Homebrew tap.
- npm bootstrapper.
- `llms.txt` / agent onboarding.
