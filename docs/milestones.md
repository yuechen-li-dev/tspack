# Milestones

- M0 Repo skeleton + contracts + golden fixtures
- M1 TypeScript manifest frontend
- M2 Go core IR loader + schema validation + diagnostics foundation
- M3 Target/dependency graph model
- M4 Import scanner + target boundary validation
- M5 Type surface validation, first pass
- M6 Lockfile model
- M7 NPM source resolver
- M8 Git + workspace/path source resolvers
- M9 Content-addressed store
- M10 Strict node_modules materializer
- M11 sync/update/check integration
- M12 pack command
- M13 why command
- M14 security/capability policy pass
- M15 hardening, fixtures, docs, and release gate

## M62f incremental package annotations

Status: implemented. `package.manifest.tsx` supports explicit `annotatePackage(<PackageAnnotations />)` mode for existing npm packages. `tspack adopt --report` discovers annotation files under simple npm workspace patterns, reports dependency classification counts, and warns when annotation intent differs from package.json sections or ranges. Future work remains separate: optional observed package refs, govern/full ownership mode, target declarations, package.json projection previews, and per-package why/security reporting.
