# Concept promotion policy

## Status

M61d defines the policy for promoting concept fragments from local incubation to curated built-in TSPack behavior. It is a design and validation policy milestone only. It does not promote Tailwind, MachinaLayout, Shadcn, Storybook, services, remote registries, marketplaces, publishing, hooks, init-time commands, package installation, JavaScript/TypeScript concept files, arbitrary patching, template inheritance, or package-manager behavior changes.

## Product thesis

Local concepts are the incubation path. Built-in concepts are curated, stable, high-confidence primitives.

Most concepts should remain local or user-authored until they prove real usage, stable semantics, deterministic generation, clear conflict behavior, check/build smoke coverage, clear ownership boundaries, documentation, and long-term maintenance value.

TSPack should not encode every possible stack permutation. It should encode reusable primitives and let explicit concept stacks compose them.

## Maturity levels

### 1. Local concept

A local concept is owned by a user, template, project, or company.

- It is inert TOML data.
- It carries no TSPack compatibility promise beyond the local template validation rules that load it.
- It may be project-specific or company-specific.
- It must pass local template validation before it can be used.
- It may prove an idea without implying future built-in status.

### 2. Fixture concept

A fixture concept is a local concept checked into TSPack test fixtures for dogfood and regression coverage.

- It remains local-template behavior, not public built-in behavior.
- It exists to prove renderer, merge, validation, and generated-project behavior.
- It may intentionally use real packages so fixtures test actual imports and build paths.
- It may be removed, renamed, or reshaped as test coverage evolves.

Current examples include the Tailwind local concept fixture, the MachinaLayout local concept fixture, and the composed Tailwind + MachinaLayout local concept fixture.

### 3. Candidate built-in concept

A candidate built-in concept is proposed for the built-in registry but is not yet public built-in behavior.

- It must satisfy the promotion checklist in this document.
- It may live behind internal tests and docs before public exposure.
- It must have an explicit owner willing to maintain the fixture and docs.
- It must prove its compatibility story before its name becomes stable.

### 4. Built-in concept

A built-in concept is a Go-embedded registry concept maintained by TSPack.

- It has a stable name.
- It has stable semantics.
- It is covered by compatibility tests.
- It is documented as public TSPack behavior.
- It must not encode arbitrary project taste outside its stated scope.
- It must preserve explicit stack semantics: no hidden insertion, listed order is priority, and hard conflicts remain hard conflicts.

### 5. Deprecated built-in concept

A deprecated built-in concept is kept for compatibility or migration.

- It has replacement guidance.
- It emits a docs or diagnostic warning when applicable.
- It remains covered enough to prevent accidental breakage during its compatibility window.
- Its removal path must be documented before removal.

## Promotion checklist

A local or fixture concept is eligible for built-in promotion only when all applicable checklist items are satisfied.

### A. Scope clarity

- The concept has one clear responsibility.
- The concept name is stable, specific, and unlikely to need semantic redefinition.
- The concept does not secretly imply unrelated stack choices.
- The concept is not merely a favorite full starter template.
- The concept boundary can be explained without listing an entire application architecture.

### B. Inertness

- The concept is data, not executable code.
- It does not define scripts or hooks.
- It does not execute commands during `tspack init`.
- It does not fetch remote content by itself.
- It does not install packages during init.
- It does not require arbitrary JavaScript or TypeScript concept files.
- It does not patch arbitrary text.

### C. Explicit stack semantics

- Expected companion concepts are declared.
- Missing companions fail with clear diagnostics.
- No companion concept is silently inserted.
- Compatible package kinds are declared.
- Conflicts are explicit and deterministic.
- Listed concept order remains the user-visible priority order.

### D. File ownership

- The concept owns only files it explicitly contributes.
- The concept does not patch arbitrary files.
- File conflicts are deterministic and diagnosed.
- The concept never silently overwrites files.
- If the concept needs config changes, it must either own the config file in a fixture/template, contribute a separate imported config file, or wait for concept-authored config projection support.

### E. Manifest semantics

- Contributions are renderable by a focused built-in renderer or the generic concept renderer.
- Unsupported contributions fail loudly instead of being dropped.
- Dependencies, tools, peers, run targets, targets, and target dependency wiring have clear ownership.
- Runtime dependencies, tools, and peer dependencies are classified semantically correctly.
- Target boundary references are valid and tested.

### F. Validation coverage

Required smoke coverage for promoted concepts:

- `tspack init`
- `tspack update`
- `tspack sync`
- `tspack check`
- `tspack check --format`
- `tspack update --policy --dry-run --json`
- a build or pack path appropriate to the concept kind

Additional coverage by package kind:

- App concepts must pass `tspack run build` or the equivalent app build path.
- Library concepts must pass relevant build, type, pack dry-run, and pack verification paths.
- Service concepts cannot be promoted until env/service renderer support and validation exist.

### G. Regression coverage

- An individual fixture test covers the concept alone.
- A composition fixture covers the concept with expected companions when composition is part of the value proposition.
- Missing companion diagnostics are tested.
- `compatibleKinds` diagnostics are tested.
- File conflict diagnostics are tested when the concept contributes files.
- Dependency, tool, peer, target, and run-target conflict diagnostics are tested when applicable.

### H. Documentation

Built-in concept docs must include:

- Purpose.
- Expected companion concepts.
- Compatible package kinds.
- Contributions.
- File ownership.
- Example stack.
- Non-goals.
- Troubleshooting.
- Any compatibility or deprecation notes.

### I. Stability and maintenance

- The upstream dependency package exists and is maintained enough for TSPack to rely on it.
- APIs and import paths used by generated files are documented, type-backed, or otherwise stable enough for fixture maintenance.
- Version range policy is intentional.
- TSPack maintainers are willing to update fixtures when upstream changes.
- The built-in concept has a clear deprecation strategy.

## Forbidden built-in behavior

Built-in concepts must not:

- Execute commands during init.
- Install packages during init.
- Fetch remote files during init.
- Depend on scripts or hooks in concept definitions.
- Use JavaScript or TypeScript concept files as executable concept logic.
- Patch arbitrary files with search/replace or AST surgery outside supported structured projections.
- Silently add other concepts.
- Silently drop unsupported manifest or file contributions.
- Hide hard conflicts behind last-writer-wins behavior.
- Claim compatibility with a package kind it has not validated.

## Anti-patterns

### Mega concept

A concept such as `full-saas-app` that secretly adds auth, database, UI, deployment, linting, CSS, tests, and dashboards has too much ownership. It should be decomposed into explicit concepts or remain a local template.

### Hidden insertion

A concept must not silently add other concepts. Companion concepts should be declared as expectations so missing pieces fail clearly.

### File patching

A concept must not search `App.tsx` and insert imports, mutate config by regex, or perform arbitrary text surgery. It should own a file, contribute an imported config file, or wait for structured projection support.

### Dependency theater

A concept must not declare a dependency that generated code does not actually use or validate. If a package is listed to imply support, fixture checks must prove the generated project can update, sync, check, and build or pack.

### Fake package or fake API

A concept must not reference a package, import path, or API that is not proven by update/sync/build validation or an equivalent smoke path.

### Matrix template explosion

TSPack should not add standalone built-ins such as `react-tailwind-machina-shadcn-storybook-auth-dashboard` for every permutation. It should add reusable primitives only when they meet the promotion bar.

### Overbroad concept

A concept such as `frontend.best-practices` is too vague because it does not define ownership, compatibility, or stable semantics.

### Silent unsupported contributions

A concept must not claim to add env, services, pack metadata, or other unsupported surfaces if the renderer drops those contributions. Unsupported contributions must fail loudly until the surface exists.

## Promotion workflow

1. Start as a local concept.
2. Add one clean smoke fixture that uses the local concept in a real generated project.
3. Add one composition fixture if the concept is expected to compose with other concepts.
4. Run the full validation set for the relevant package kind.
5. Document ownership, contributions, compatibility, and non-goals.
6. Promote only if the concept is broadly useful and stable enough to maintain.
7. Add the built-in registry entry.
8. Keep the local fixture as regression coverage or convert it to a built-in fixture with equivalent coverage.
9. Add migration or deprecation notes if the built-in replaces a local example or older concept.

## Do not promote when

- The concept is company-specific branding or project-specific policy.
- The upstream package or API is unstable enough that fixtures would churn frequently.
- The concept is useful for only one application.
- The concept requires unsupported config patching.
- The concept requires env, service, or pack support before those renderers exist.
- The concept would force too many stack opinions into one name.
- The concept cannot pass the validation checklist without brittle exceptions.

## Avoiding template matrix explosion

TSPack should prefer primitive concepts over combination templates. A template may name a useful concept stack, but built-in promotion should happen at the primitive level unless the combination itself has a narrow, stable product meaning.

The preferred path for combinations is explicit composition:

```toml
concepts = [
  "react.app",
  "browser.spa",
  "vite.app",
  "typescript.app",
  "my-company.tailwind",
  "my-company.machina-layout",
]
```

This keeps stack choices inspectable, conflict behavior deterministic, and generated output attributable to named concept owners.

## Inspectability and trust

Users and LLMs should be able to inspect a concept stack and understand generated project intent without executing code.

A trustworthy concept stack should expose:

- The ordered concept list.
- Which concepts are built-in and which are local.
- Expected companions and conflicts.
- Compatible package kinds.
- Manifest contributions by concept.
- File contributions and owning concepts.
- Unsupported contributions as explicit diagnostics.
- Documentation for promoted built-ins.

This policy preserves the core trust model: concepts are inert, explicit, deterministic inputs to generation, not hidden installers or starter-kit scripts.
