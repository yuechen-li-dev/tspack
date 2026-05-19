# `tspack doctor` (M27)

`doctor` reports local environment readiness for TSPack without mutating the project.

## Commands

- `tspack doctor`
- `tspack doctor format`
- `tspack doctor run`
- `tspack doctor inspect`
- `tspack doctor --json`
- `tspack doctor --root <path>`

## What doctor checks

- Project basics: root, `manifest.tsx`, `ts-lock.toml`, `node_modules`.
- Format/lint readiness: Biome backend resolution and config presence.
- Run readiness: runtime executables and declared run targets.
- Inspect readiness (**experimental**): environment suitability and explicit-backend requirements.

## What doctor does not do

- No installs/downloads (`npm`, `npx`, browser binaries, OS packages).
- No run-target startup.
- No port scanning.
- No auto-attachment to running apps.
- No package-manager mutation.

## Status meanings

- `ok`: ready/available.
- `warning`: optional capability missing or experimental limitations.
- `error`: required capability missing for selected scope.
- `not_applicable`: check is informational and requires explicit user input.
