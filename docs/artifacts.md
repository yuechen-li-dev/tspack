# Standalone native artifacts (M19a)

Standalone artifacts are suite-level native xTest generators declared as `<Artifact>` children of `<Suite>`.

## Standalone vs test artifacts

- **Standalone artifacts**: `<Suite><Artifact ...>{callback}</Artifact></Suite>` and run via `tspack artifact`.
- **Test artifacts**: `<Fact>/<Theory>` child artifacts and run during `tspack test`.

These execution paths are intentionally separate:
- artifact mode does not run Facts/Theories
- test mode does not run standalone Artifacts

## Declaration rules

Suite-level `<Artifact>` requires:
- `name` string literal
- `path` string literal
- callback body

Suite-level `<Artifact>` rejects:
- `optional` prop (`TSPACK_ARTIFACT_OPTIONAL_NOT_ALLOWED`)
- missing body (`TSPACK_ARTIFACT_MISSING_BODY`)
- unsafe path (`TSPACK_ARTIFACT_INVALID_PATH`)

`format` is metadata only in M19a; no Octagon parser/writer is included.

## Command

```bash
tspack artifact --root .
tspack artifact --root . --list
tspack artifact --root . --filter manifest-ir
tspack artifact --root . --out .tspack/artifacts
tspack artifact --root . --json
```

- Default output root: `<root>/.tspack/artifacts`
- Each artifact writes under a sanitized per-artifact directory.

## Writer behavior

Artifact callbacks receive `artifact` with:
- `writeText(name, text, reason)`
- `writeJson(name, value, reason)`
- `writeBytes(name, bytes, reason)`

Write reason is mandatory. Written outputs record path/hash/size/reason.

## Pack distinction

- `tspack pack` creates package `.tgz` archives.
- `tspack artifact` runs native standalone artifact generators.
