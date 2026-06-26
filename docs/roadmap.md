# TSPack Roadmap

This roadmap records known post-`0.1.0` work without turning the first release into a stability promise.

## Post-0.1.0 planned

- Phase 11 inspect/browser deep testing.
- Policy-driven update mutation.
- Targeted policy planning.
- React/single-version coherence policy.
- `check --lint`.
- Per-file format diagnostics.
- Pre-commit hook generation.
- `setup-tspack` hosted smoke after the first public release exists.
- Homebrew, mise, and npm bootstrapper distribution.
- `get.tspack.dev` hosted installer.
- Visionary / VS Code fork work.

## Non-goals for 0.1.0

- Replacing all npm scripts.
- Zero-bootstrap self-hosting.
- Production stability claims.
- New package-manager, resolver, lockfile schema, lifecycle execution, or security model behavior.

## Template roadmap

M54a added the concept-aware inert template engine and built-in static template. M54b/M54d added React app and React library built-ins. M55a completes the internal Template IR normalization path for built-in and local templates while preserving current behavior. Remote templates, registries, and interactive prompts remain later work.

## Template roadmap notes

M54b added the built-in React + Vite app template, and M54d added the React library template. Future work remains concept overlays, UI-library overlays, Next.js, Vue, Tailwind, router-enabled templates, concept validators, remote templates, and package-manager-specialized templates.


## Template overlay future

Future template work should add concept overlays rather than hardcoded template explosion. Planned overlays include `ui.mui`, `ui.shadcn`, `ui.antd`, `style.tailwind`, and `router.react-router`. A Next.js template remains future work and is intentionally separate from the M54d React library template.
