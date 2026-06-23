# Embedded bridge assets

`generated_assets.go` is produced by `go run ./tools/generate-embedded-bridges` for release builds that use the `tspack_embedded_bridges` build tag.

The generated file embeds `manifest-frontend/dist` bridge entrypoints in the release binary and is intentionally ignored by Git. Do not commit copied JavaScript bridge bundles or generated Go asset blobs.
