# Concept fragment composition design

## Status

This began as the M60a design document. M60b adds an internal implementation slice, but the design remains non-user-facing.

M60b adds no runtime behavior, template behavior, command behavior, package-manager behavior, built-in template migration, local custom concept support, remote concept support, or public concept fragment authoring surface.


## M60b implementation note

M60b adds the first internal implementation slice for concept fragment composition. The implementation is intentionally Go-embedded and internal: `internal/concepts` defines inert fragment records, a deterministic built-in registry, concept graph resolution, `MergedConceptIR`, conservative merge laws, and conflict diagnostics that name the contribution path and concepts involved.

This is infrastructure only. Existing static, React app, and React library templates are not migrated to fragment-driven rendering in M60b, and no public custom concept format or CLI surface is added. The current template pipeline and generated output remain behavior-preserving while tests prove that the current static, React app, and React library concept compositions can resolve and merge into semantic IR.

## Thesis

Concepts are not just labels. They should become inert, declarative, structured fragments that describe a piece of project genesis intent.

Templates should become named concept compositions. A template provides identity, variables, and an ordered list of concepts. The concepts contribute dependencies, tools, manifest structure, projections, files, starter slots, diagnostics, and compatibility constraints. The engine then merges those fragments into a semantic project shape before producing a concrete write plan.

`TemplateIR` is the composition boundary. Template metadata and future local concept definitions lower into `TemplateIR`-level intent before planning. `TemplatePlan` is the write boundary. It is the first layer that should contain concrete rendered paths and file writes for an invocation.

The primary design principle is:

> Concept fragments are the generative unit. Templates are named concept compositions.

This deliberately avoids starting from a base-template-plus-overlays model. A React + MachinaLayout app should be expressible as `react.app` plus `machinalayout.layout`, not as a copied React template with a second template recursively applied on top.

## Non-goals

M60a does not design or implement these as active product behavior:

- Overlays as the primary model.
- Command execution by concepts.
- Remote concepts.
- Recursive template invocation.
- Turing-complete template metaprogramming.
- Package installation during `tspack init`.
- Migration of existing built-in templates.
- Local custom concept execution.
- Arbitrary code in concept fragments.
- Uncontrolled regex or string surgery against generated files.

## Current problem

The current templates are inert and concept-aware, but the concepts are metadata. The built-in `static`, `react`, and `react-library` templates still mostly own their generated manifests and files.

That creates several problems:

- `static`, `react`, and `react-library` duplicate template knowledge around workspace shape, manifest boundaries, TypeScript configuration, Vite configuration, package compatibility files, update policy rows, and security acknowledgements.
- React dependencies, React type tooling, JSX TypeScript settings, Vite tooling, and starter source files are repeated across templates that share the same semantic intent.
- Future combinations such as Tailwind, Shadcn, MachinaLayout, Storybook, routers, backend services, test runners, and UI libraries would create a combinatorial template explosion if each complete combination needs a copied template.
- Local custom templates currently require copying too much generated template content when users only want to add or swap one semantic feature.

The design target is to make the reusable unit a concept fragment rather than a whole template directory.

## Proposed model

### Template

A template is a named composition:

```toml
format = 1
name = "react-machina"
description = "React + MachinaLayout app"
kind = "app"
concepts = [
  "tspack.workspace",
  "tspack.manifestBoundary",
  "tspack.securityPolicy",
  "tspack.updatePolicy",
  "typescript.app",
  "vite.app",
  "react.app",
  "machinalayout.layout",
  "browser.spa",
]
```

A template owns:

- Identity: name, description, kind, and future display metadata.
- Variables: project name, package name, runtime, and template-specific parameters.
- Ordered concept list: the user's declaration of intended project capabilities.

A template should not invoke another template. It should not recursively generate a second template. It should only declare concepts and variables that lower into the same composition pipeline as built-ins.

### ConceptFragment

A `ConceptFragment` is an inert structured contribution. It can declare what it provides, what it requires, what it conflicts with, and the semantic pieces it contributes.

### ConceptGraph

A `ConceptGraph` validates the selected concept list, resolves declared requirements, detects conflicts, and produces a deterministic concept order for merging. It is graph resolution, not recursive template expansion.

### MergedConceptIR / ProjectGenesisIR

The merged IR is the semantic project shape produced by combining concept fragments. It should describe project intent such as dependencies, tools, package kind, RunTargets, Env contracts, Service requirements, tsconfig fields, Vite config fields, starter slots, and generated manifest structure without yet being a concrete file write plan.

`MergedConceptIR` and `ProjectGenesisIR` are candidate names. The important property is that the merged object is semantic and structured, not a bag of already-rendered text patches.

### TemplatePlan

`TemplatePlan` remains the concrete rendered file and write plan. Path validation, destination collision checks, and apply-time safety stay centralized at the plan/apply boundary.

## Pipeline

Current pipeline:

```text
RawTemplate -> TemplateIR -> TemplatePlan -> ApplyPlan
```

Future pipeline:

```text
RawTemplate
-> TemplateIR
-> ConceptGraph
-> ConceptFragments
-> MergedTemplateIR / ProjectGenesisIR
-> TemplatePlan
-> ApplyPlan
```

The future pipeline preserves the current separation between parsing, normalization, planning, and applying. It inserts concept graph resolution and fragment merging before concrete file rendering.

## Concept fragment shape

A possible internal shape is:

```text
ConceptFragment {
  name
  version maybe later
  description

  providesConcepts[]
  requiresConcepts[]
  requiresAnyOf[] maybe later
  optionalConcepts[]
  conflicts[]
  compatibleKinds[] maybe later

  variables[]

  manifestContributions
  fileContributions
  projectionContributions
  starterSlots

  diagnostics
  warnings
}
```

This shape is intentionally declarative. It describes intent and constraints. It does not run code.

### Manifest contributions

Manifest contributions may include:

- Workspace fields.
- Package kind.
- Dependencies.
- Tool dependencies.
- Peer dependencies.
- Targets.
- RunTargets.
- Env declarations.
- Service requirements.
- Update policy rows.
- Security policy acknowledgements.
- Pack metadata.
- Concepts to preserve in output metadata.

Examples:

- `tspack.workspace` contributes workspace shell, name/runtime variables, and baseline manifest shape.
- `typescript.app` contributes TypeScript tool dependency, app-oriented tsconfig projection, and update policy rows.
- `vite.app` contributes Vite tool dependencies, Vite config projection, and `dev`, `build`, and `preview` RunTargets.
- `react.app` contributes `react` and `react-dom` dependencies, React type tools, React JSX settings, and starter app slots.

### File contributions

File contributions may include:

- Add a static file.
- Add a rendered file.
- Add a generated file from a structured projection.
- Claim a starter slot.
- Append to a named slot.
- Fail on path conflict.

A file contribution should describe the destination and source of truth. It should not patch arbitrary existing text by uncontrolled regex/string surgery.

### Projection contributions

Projection contributions may include:

- `tsconfig` fields.
- `vite.config.ts` fields.
- `biome.json` fields.
- `package.json` compatibility metadata.
- README sections.

Projection contributions are structured objects or typed projection records. They merge with deterministic rules and lower to generated files at planning time.

### Starter slots

Starter slots are named extension points for files whose content is expected to vary by concept composition. Candidate slots include:

- `app.entry`
- `app.rootComponent`
- `app.styles`
- `readme.sections`
- `package.exports`
- `test.examples`

A concept can fill, claim, or append to a slot according to the slot's declared merge mode. Slot conflicts should be explicit and diagnosable.

## Merge laws

Merging must be deterministic, stable, and conservative. Implicit replacement is not allowed. If two fragments want incompatible ownership of the same semantic path, the merge fails with a diagnostic that names the concepts and contribution path.

### Concepts

- Concept names are unioned.
- Output order is stable and deterministic.
- The template's declared concept order is preserved where possible, with required dependencies inserted by stable topological order.

### Variables

- The same variable with the same default, description, constraints, and source is accepted.
- Compatible duplicate declarations may coalesce.
- Incompatible definitions are a conflict.

### Dependencies, tools, and peers

- The same package key, dependency kind, source, and range is accepted.
- The same package key with a different range is a conflict unless a future explicit resolver policy exists.
- The same package key in different dependency kinds is a conflict or must follow a future explicit rule. For example, `react` as a runtime dependency and `react` as a peer dependency should not silently merge.

### RunTargets

- The same target name with identical command, cwd, env, service requirements, readiness, and metadata is accepted.
- The same target name with different command, env, service, readiness, or runtime is a conflict.
- Future override semantics must be explicit, not implicit.

### Env declarations

- The same env name with identical `required`, `default`, `secret`, and description metadata is accepted.
- Differences in required/default/secret semantics are conflicts unless a future explicit merge rule exists.
- Secret defaults and secret values must never appear in diagnostics.

### Service requirements

- The same service requirement with identical name, protocol, endpoint, optionality, and preflight metadata is accepted.
- The same service name with a different endpoint or incompatible preflight contract is a conflict.

### Update and security policy

- Policy rows are additive.
- Duplicate identical rows are accepted.
- Rows that apply incompatible policy to the same package/source/lifecycle category are conflicts.
- Security acknowledgements are additive but must not silently weaken existing policy.

### Files

- The same path with identical content is accepted.
- The same path with different content is a conflict.
- Explicit slot ownership can avoid direct file conflicts by allowing fragments to contribute to a generated file through structured slots.
- Path safety remains enforced by `TemplatePlan` and `ApplyPlan`.

### Structured projections

- Objects merge recursively with deterministic conflict rules.
- Scalar fields must be identical unless a field-specific rule allows composition.
- Arrays append with deduplication only where semantically safe and order-stable.
- Projections never mutate arbitrary text by uncontrolled regex or string patching by default.

### Starter slots

- Slots have declared merge modes such as `singleOwner`, `appendOrdered`, or `mapByKey`.
- A `singleOwner` slot fails if more than one concept claims it.
- An `appendOrdered` slot preserves deterministic concept order and deduplicates only where the slot declares a safe key.
- A `mapByKey` slot fails on same key with different value.

## Ordering

The template concept list declares user intent order. It is the primary stable ordering input.

The engine then builds a dependency graph from `requiresConcepts` and any future declarative requirement fields. It resolves that graph with a stable topological sort:

1. Preserve the template's listed order where dependencies allow it.
2. Insert required concepts before dependents.
3. Use a stable registry order only as a tie-breaker for implicit required concepts.
4. Report cycles with the concept names and requirement edges.
5. Report conflicts with concept names and contribution paths.

Concept fragments are merged in that resolved order. Output is deterministic for the same template metadata, fragment registry, variable inputs, and TSPack version.

## Compatibility and constraints

Concept constraints should stay declarative. Candidate fields include:

- `requiresConcepts`: all listed concepts must be present or resolvable.
- `requiresAnyOf`: at least one concept from each group must be present or resolvable.
- `conflicts`: listed concepts cannot appear in the same composition.
- `compatibleKinds`: template or package kinds where the concept can apply.
- `targetPackageKind`: package kind the concept contributes or requires.
- `optionalConcepts`: concepts that enable extra contributions if present but are not pulled in automatically.

Examples:

- `vite.app` requires `typescript.app`.
- `react.app` requires `typescript.app` and likely a browser app concept such as `browser.spa` or a declared app host slot.
- `machinalayout.layout` requires `react.app`.
- `react.library` conflicts with `browser.spa` because a reusable library should not own a SPA document entry.
- `node.service` requires a service-oriented TypeScript concept and package kind `service`.
- `nestjs.service` requires `node.service` and contributes NestJS dependencies, service RunTargets, and starter files.

Constraints are validation inputs. They are not scripts and they do not perform runtime probing.

## Local custom concepts

Local custom concepts are a future design target, not M60a behavior.

A future local template could look like:

```toml
format = 1
name = "my-company-react"
description = "Company React starter"
kind = "app"
concepts = [
  "tspack.workspace",
  "typescript.app",
  "vite.app",
  "react.app",
  "my-company.auth-client",
  "my-company.design-system",
]

[[concepts]]
name = "my-company.auth-client"
from = "./concepts/auth-client.toml"

[[concepts]]
name = "my-company.design-system"
from = "./concepts/design-system.toml"
```

Important constraints:

- Local custom concepts are inert files.
- Local custom concepts do not run scripts.
- Local custom concepts do not fetch remote content.
- Local custom concepts lower into the same IR as built-in concepts.
- Local custom concepts cannot bypass variable validation, path safety, file conflict checks, or write boundaries.
- Local custom concepts cannot execute arbitrary code during init.

Local custom concepts should allow organizations to express small semantic additions without copying a whole generated React/Vite template.

## Built-in template migration path

A conservative migration path is:

- **M60a:** design only.
- **M60b:** add an internal concept fragment model and built-in registry for a tiny subset. No public custom concepts yet.
- **M60c:** migrate static template manifest generation to concepts.
- **M60d:** migrate React app template generation to concepts.
- **M60e:** migrate React library template generation to concepts.
- **M60f:** add local custom concept fragments.

Future concepts after the migration path may include:

- `style.tailwind`
- `machinalayout.layout`
- `docs.storybook`
- `ui.shadcn`
- `node.service`
- `nestjs.service`

Each phase should preserve existing template output or intentionally document any output change. Migration should happen behind the internal pipeline before exposing local custom concept authoring.

## Examples

### Current `static` template as a concept composition

```toml
concepts = [
  "tspack.workspace",
  "tspack.manifestBoundary",
  "tspack.securityPolicy",
  "tspack.updatePolicy",
  "typescript.app",
  "vite.app",
  "browser.static",
]
```

### Current `react` template as a concept composition

```toml
concepts = [
  "tspack.workspace",
  "tspack.manifestBoundary",
  "tspack.securityPolicy",
  "tspack.updatePolicy",
  "typescript.app",
  "vite.app",
  "react.app",
  "browser.spa",
]
```

### Current `react-library` template as a concept composition

```toml
concepts = [
  "tspack.workspace",
  "tspack.manifestBoundary",
  "tspack.securityPolicy",
  "tspack.updatePolicy",
  "typescript.library",
  "vite.library",
  "react.library",
  "package.exports",
  "package.peerDependencies",
  "tspack.pack",
]
```

### Future NestJS service example

```toml
concepts = [
  "tspack.workspace",
  "typescript.service",
  "node.service",
  "nestjs.service",
  "test.vitest",
  "format.biome",
]
```

### Future MachinaLayout addition

```toml
concepts = [
  "react.app",
  "machinalayout.layout",
]
```

The MachinaLayout concept would contribute its dependency, a starter `LayoutRow[]` example, and a claim or fill for the app starter slot. It would not require copying the entire React template.

## Safety model

Concept fragment composition must remain inert and declarative:

- No scripts.
- No command execution.
- No package install during init.
- No remote concepts.
- No recursive template invocation.
- No recursive concept expansion beyond graph resolution.
- No arbitrary code.
- No uncontrolled text mutation by default.
- Concepts are structured data that lower into IR.
- Path safety remains a `TemplatePlan` and `ApplyPlan` responsibility.
- Existing destination conflict behavior remains centralized at the write boundary.
- Secret values never appear in generated diagnostics.
- Local custom concepts cannot bypass safety/path rules.

The design should improve reuse without weakening TSPack's existing inert template posture.

## Open questions

- Should concept fragments be authored as TOML, TSX, JSON, or Go-embedded structs first?
- Should built-in fragments be authored as TOML or Go data?
- How much of `manifest.tsx` should be generated from structured IR versus template text?
- How do we represent `vite.config.ts` safely without creating a code-generation trap?
- What is the minimum useful slot system?
- Do we need explicit override or replace semantics, and if so where are they safe?
- Should concept fragments have versions in the initial implementation?
- How exactly do update policy rows merge when package ranges differ?
- Should local custom concepts be allowed before all built-in templates migrate?
- How do we prevent concept drift from becoming too abstract or detached from real generated projects?


## M60c implementation note: static manifest migration

M60c moves the built-in `static` template's `manifest.tsx` generation onto the internal concept fragment engine. The static template now resolves its built-in concept list through `internal/concepts`, merges the fragments into `MergedConceptIR`, and renders the manifest from that merged concept intent during template planning.

The migration is intentionally narrow:

- only the built-in `static` template manifest is concept-rendered;
- static non-manifest files still use the existing inert template file projections;
- `react`, `react-library`, and local templates continue to use their existing file projection path;
- no public concept CLI behavior, TOML-authored concepts, local custom concepts, overlays, remote concepts, scripts, or package install behavior is added.

The expected static composition is:

```text
tspack.workspace
tspack.manifestBoundary
tspack.securityPolicy
tspack.updatePolicy
typescript.app
vite.app
browser.static
```

From the user's point of view, `tspack init --template static` remains the same scaffold shape; the manifest is now backed internally by concept fragments so later React and library migrations can follow the same boundary.
