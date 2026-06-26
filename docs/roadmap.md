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

## CLI help future

M56a keeps help output human-readable and deterministic. Structured JSON help such as `tspack help commands --json` or `tspack help init --json` remains future work rather than a release blocker for v0.1.3.

## Experimental Python module design

M57a records an exploratory Python module/domain design in `docs/design/python-module-experiment.md`. The direction is one manifest frontend with multiple project/package backends: `manifest.tsx` remains the universal typed contract, while TypeScript/npm remains the product focus today and Python/PyPI stays experimental future work.

This is not Python support yet. The roadmap explicitly excludes `manifest.py`, executable Python project configuration, PyPI resolver implementation, `uv` integration, lockfile renaming, packaging publication, and changes to existing TypeScript/npm behavior for this milestone.

Future Python work should be high-locality: Python-specific behavior belongs behind an ecosystem/backend seam, not as scattered `pypi` conditionals across the TypeScript/npm resolver, run, security, lockfile, pack, template, and CLI paths.

## M57b ecosystem/backend seam spike

M57b adds a small ecosystem/backend seam spike in `docs/design/ecosystem-backend-seam.md` plus internal vocabulary for the production TypeScript/npm backend and reserved Python-family/PyPI design terms. The intent is high locality: future ecosystem behavior should live behind backend-owned validation/resolution/materialization/security/runtime seams instead of scattered conditionals. The seam separates package ecosystem, backend, runtime family, runtime implementation, environment/materialization strategy, execution/build mode, version/range scheme, and security/build risk so CPython, PyPy, Cython/native-extension, Triton, Mojo, Numba, and JAX-style systems do not collapse into one flat `python` runtime.

This remains architecture groundwork only. TSPack still has no Python runtime, Python resolver, PyPI source acceptance, uv integration, Python templates, pyproject projection, `manifest.py`, `runtime: "python"`, lockfile rename, or package-manager behavior change. TypeScript/npm remains the only production behavior, and venv-like layouts are future implementation details rather than user-facing project model.
