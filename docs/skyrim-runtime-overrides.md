# Skyrim machine-local runtime overrides

The committed `manifest.tsx` and runtime TOML are the portable, safe source of
truth. A Skyrim target may opt in to a small run-scoped variation by declaring
`runtimeOverrideTarget` and typed `runtimeOverrideFields`. Each field is an
explicit TOML leaf and has a `boolean`, `string`, or `integer` type. There is
no general TOML patch syntax.

For example, Marionette declares only its presenter enabled flag, semantic
actuation flag, and presenter profile. Its committed TOML stays disabled.

Machine-local values belong in the ignored `.tspack/skyrim-hosts.toml` next to
the selected host:

```toml
[hosts.skyrim-dev.runtimeOverrides.MarionetteSSE]
host = "skyrim-dev"
"eternal_dragonborn.development.presenter.enabled" = true
"eternal_dragonborn.development.presenter.allow_semantic_actuation" = false
"eternal_dragonborn.development.presenter.profile" = "skyrim-dev"
```

The target name and `host` identity must match the selected `SkyrimTarget`.
Unknown targets, unknown or undeclared paths, duplicate TOML keys, values of
the wrong type, and malformed source paths are rejected before deployment.
Fields marked `secret` in the manifest are represented as `<redacted>` in both
human and JSON reports.

`tspack run skyrim --dry-run --json` parses the committed TOML and local
profile without writing files. It reports the committed and effective SHA-256
hashes, declared fields, redacted applied values, and the planned restoration.
For a real run, TSPack deterministically derives the effective TOML in ignored
`build/skyrim/runtime/`; the authored TOML is never modified. It preserves the
authored document and changes only the declared scalar assignment lines, which
keeps parser-specific syntax and unrelated comments intact.

When a local value actually changes the effective configuration, TSPack deploys
that generated file in the normal atomic transaction. It keeps the command
alive until SKSE has exited, then performs a second transactional deployment of
the committed safe TOML. The restoration result and final hash are written to
the materialization report. If startup or verification fails, TSPack also
attempts that restoration before returning the lifecycle failure. The rollback
directory stays under `build/skyrim/rollback`, outside `Game/Data`.

Do not create a copied local manifest or manually edit the deployed TOML. The
declared profile is the reviewable ownership boundary: portable defaults remain
in source control, and local experimental variation remains ignored,
validated, reported, and temporary.
