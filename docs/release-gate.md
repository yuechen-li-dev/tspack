# Release gate (M24 command surface)

## Smoke command checklist

- `tspack init --kind <library|app> --name <package-name>`
- `tspack check`
- `tspack update`
- `tspack sync`
- `tspack why <dep>`
- `tspack how --list`
- `tspack how TSPACK_IR_INVALID_RELATIVE_PATH`
- `tspack pack`
- `tspack test`
- `tspack artifact`
- `tspack bench`
- `tspack doom`
- `tspack run`
- `tspack inspect <target>` (**experimental**)

## Mutation expectations

- `update` mutates `ts-lock.toml` and populates required store artifacts.
- Release smoke must pass `tspack update --root <fixture>` followed by `tspack sync --root <fixture>` using a local/fake registry fixture.
- `check`, `sync`, `pack`, `why`, `run`, and `inspect` do not mutate `ts-lock.toml` unless explicitly documented.
- `run` and `inspect --run` do not mutate manifest contract files.
- `artifact`, `test`, `bench`, and `doom` may write harness outputs/artifacts but do not rewrite manifest/lock contract state.

## Non-goal checks

- Unsupported command examples (`build`, `dev`, `publish`, `install`) must fail deterministically.
- `run` and `inspect` must not infer `package.json` scripts when no declared `RunTargets` exist.


- `tspack format` and `tspack lint` are Biome-backed lifecycle UX commands. See `docs/format-lint.md`.

- `tspack doctor` adds non-mutating environment diagnostics. See `docs/doctor.md`.

## Manifest frontend build scope

- `npm run build` in `manifest-frontend/` validates production source files only (`src/index`, `src/cli`, and `src/inspect/*`).
- `npm test` in `manifest-frontend/` remains responsible for executing frontend tests.
- `npm run typecheck:manifest-api` in `manifest-frontend/` validates `tspack/manifest` authoring declarations against typed fixtures.
- Stricter standalone test-file typecheck is tracked as future M31c work.
