# M71 compiler target boundary

## Law and authority

The compiler owns the language.

TSPack owns the project around the compiler.

Standalone compiler operation may observe the local environment, but does not become a package manager.

TSPack-managed compilation consumes resolved project truth rather than rediscovering it.

Compiler configuration is compiler-owned; compiler orchestration is TSPack-owned.

In particular, TSPack is authoritative for full `manifest.tsx` realization: dependency declarations, Requirement Tape resolution, registries and source policy, exact versions, peers, aliases, mirrors, lock state, materialization, tool acquisition, security, lifecycle, and build/run orchestration. `tsconfig.tsx` belongs to Copeland. TSPack may locate, fingerprint, cache-key, and pass that file, but does not interpret its compiler options, project mappings, or `tsc`/`tscl` source ownership.

The prior ambiguity was concrete: `internal/cli/build_command.go` directly serialized a Copeland-shaped request, selected one package-wide compiler, walked `src` as though TSPack owned Copeland's internal source partition, and rejected `tsc` builds. Copeland's resolver accepted either a manifest or an unversioned `.request.json` through one method. M71 replaces the cross-repository payload with a versioned generic descriptor plus a bounded `copeland-v1` payload. Package-level `compiler` remains a compatibility default; target-level identity is authoritative.

## CompilerTarget IR

`internal/compiler` owns the small boundary model and no language semantics. A target separates:

- target package/name;
- language identity;
- compiler implementation and version;
- compiler tool source/name/version/path;
- opaque compiler config reference and fingerprint;
- candidate project inputs and fingerprints;
- resolved compiler-visible package bindings;
- runtime family/name;
- semantic outputs;
- requested capabilities;
- an optional bounded, versioned compiler payload.

Compiler and runtime are deliberately independent. `tsc` with Node, Copeland emitting JavaScript for Node or a browser, and a future native compiler are different combinations rather than values in one `compiler` string. Selection is per target, so one package or workspace may contain `frontend -> tsc` and `domain -> tscl` without a global compiler choice.

The capability vocabulary is semantic and intentionally small: parse, type-check, JavaScript/declaration/native/object/Wasm emission, incremental compilation, project references, source maps, compiler-owned config, and compiler-owned source partitioning. It is not a universal flag schema.

## Adapter and invocation model

An adapter describes capabilities, validates a target, and prepares an invocation. It cannot resolve packages or interpret a type system.

- The `tsc` adapter requires `language=typescript`, `compiler=tsc`, the project-managed TypeScript tool, and an opaque `tsconfig.json` reference. `tspack build` now invokes this real path and verifies the declared JavaScript artifact.
- The Copeland adapter requires `language=copeland-ts`, `compiler=tscl`, and payload `copeland-v1@1`. It passes the descriptor to the independently built Copeland tool. Copeland applies `tsconfig.tsx` source ownership and language semantics.

Compiler invocation produces artifacts; it is not a `RunTarget`. Run targets and service lifecycle remain runtime intent that may consume those artifacts.

## Descriptor protocol

The JSON protocol has `schemaVersion: 1` and explicit `target`, `language`, `compiler`, `tool`, `compilerConfig`, `sources`, `packages`, `runtime`, `outputs`, `capabilities`, and optional `compilerPayload` sections. These DTOs are distinct from the in-memory Go IR.

Package bindings carry semantic identity, exact selected version, materialization path, Node-compatible materialization name, local alias, role, and optional type surfaces. For example, `jsr:@std/path` remains semantically distinct from `@jsr/std__path`; an npm alias retains both its local import name and `npm:` source identity. Requirement Tape history is not transported. The compiler receives resolved truth only.

The Copeland payload contains only Copeland's entry/emission and static npm contract projection. It does not grant registry, solver, lockfile, peer, mirror, fallback, lifecycle, or materialization authority. Unknown generic additive fields are ignored by Copeland's JSON reader; unknown descriptor versions, missing required identity, and incompatible payload versions are rejected explicitly.

Cache-relevant descriptor facts include compiler/tool version, config fingerprint, source fingerprints, resolved package bindings, runtime, and outputs. Copeland then derives its semantic context fingerprint from the selected compiler-owned sources, project type profile, runtime, and package bindings. Unrelated TSPack policy is excluded.

## Ownership matrix

| Concern | Bare Copeland | TSPack + Copeland | TSPack + tsc | Future TSPack + alternate compiler |
| --- | --- | --- | --- | --- |
| language parser | Copeland | Copeland | TypeScript | compiler |
| type system | Copeland | Copeland | TypeScript | compiler |
| compiler config | Copeland `tsconfig.tsx` | Copeland; TSPack passes/fingerprints | TypeScript `tsconfig.json`; TSPack passes/fingerprints | compiler |
| source ownership | Copeland config | target candidates by TSPack; internal partition by Copeland | target by TSPack; config expansion by tsc | split at target/compiler seam |
| manifest parsing | bounded Copeland profile | TSPack | TSPack | TSPack |
| dependency declarations | observed compiler-relevant subset | TSPack | TSPack | TSPack |
| dependency resolution | none; verify local presence | TSPack | TSPack | TSPack |
| registries | none | TSPack | TSPack | TSPack |
| lockfile | none | TSPack | TSPack | TSPack |
| materialization | none | TSPack | TSPack | TSPack |
| compiler acquisition | standalone distribution | TSPack tool truth | TSPack managed `typescript` | TSPack |
| compiler invocation | Copeland CLI/MSBuild | TSPack via Copeland adapter | TSPack via tsc adapter | adapter |
| runtime execution | standalone host | TSPack runtime layer | TSPack runtime layer | TSPack runtime layer |
| RunTargets/services | observed only | TSPack | TSPack | TSPack |
| LSP | standalone context | resolved descriptor context | managed TypeScript SDK | compiler language service |
| CI/deploy future | compiler artifacts only | TSPack orchestration | TSPack orchestration | TSPack orchestration |

Diagnostics follow the same division. Syntax, typing, lowering, compiler config, and internal ownership are compiler diagnostics. Missing tools/config, denied source policy, unresolved packages, unsupported capability, ambiguous target selection, and descriptor transport are project diagnostics.

## Repository and future-tool independence

Neither repository references the other's source. Local dogfood points at a built `tscl` executable but uses the same descriptor and CLI boundary as a published tool. The protocol is the compatibility surface.

ScriptC could be added with a new identity, capabilities, acquired tool, config reference, invocation, outputs, runtime, and bounded payload without modifying core target semantics. The same is true for PerryTS. Nothing in generic IR requires JavaScript output, Node, .NET, `tsconfig`, or Copeland ownership. Native link information, if eventually required, belongs in a versioned compiler payload or semantic artifact, not new Copeland fields in the core.

M71a exercises that claim with a bounded ScriptC native-executable target. The
generic target model remains language-neutral; ScriptC flags live in a
`scriptc-v1` payload, while explicit input ownership, artifact dependencies,
native output, compiler metadata, and invocation environment remain generic
orchestration facts. See [M71a ScriptC hot paths](m71a-scriptc-hotpath.md).

## Baseline and validation note

TSPack began M71 clean at `920a9f2` on `main`; its Go, manifest frontend, and VS Code baselines passed. Copeland began clean at `2b404be` on `main`. Its broad baseline already failed one Markdown Roslyn-boundary assertion and two C# byte-stability fixtures; M71-focused project-context, CLI, LSP, and integration tests are evaluated separately from those pre-existing failures.
