# M69 dependency editing closeout

## Status

M69 is complete. Native TSPack dependency intent can be added, replaced, made
optional, selected by semantic kind/source, targeted to a workspace package,
previewed, reported as JSON, and removed without hand-editing ordinary owned
dependency declarations.

## Final architecture

```text
manifest.tsx / package.manifest.tsx
  -> manifest frontend dependency authoring IR
  -> deterministic source-qualified dependency tape
  -> pure semantic add, replace, or remove edit
  -> guarded source-preserving dependency-island projection
  -> ordinary project update and resolver
  -> exact ts-lock.toml truth
```

M69a introduced the authoring IR, provenance, precedence, source-qualified
identity, authority, editability, and semantic edit operations. M69b added the
AST-proved owned dependency island and guarded atomic projection with source
format preservation. M69c exposed add with stable constraint selection and
normal resolver integration. M69d exposed removal, unshadowing, direct versus
resolved truth, and no-op removal. M69e made both commands one product family:
explicit npm source selection, kind-aware removal, package name/path targeting,
current-directory inference, replacement reporting, shared JSON/performance
conventions, source-qualified human identity, and aligned help/error guidance.

## Deliberate semantic boundaries

`--dev` is not npm `devDependencies`. TSPack's normalized `test` kind remains
reserved: it has neither a native manifest helper nor an execution contract.
`tspack add --dev` therefore fails explicitly instead of writing inert intent.

Likewise, a usable TSPack tool is a `tool(...)` dependency selected through
`<Tools>`. The M69 projector owns package dependency arrays but does not own or
rewrite `<Tools>`. `tspack add --tool` fails with the two required authoring
steps instead of resolving an unselected tool. Removal can select existing
tool/test declarations with `--kind`, `--tool`, or `--dev` because removing one
owned dependency element does not need to synthesize another manifest surface.

Dry-run remains truthful rather than artificially symmetric. Add can report
registry version selection. Remove plans semantic and source changes in memory,
but post-edit resolved truth remains unknown because ordinary update evaluates
filesystem source. An in-memory frontend/update path would require a broader
virtual-source design; M69 does not write temporary source to simulate it.

## Source identity and future registries

The preferred public selector is `--source npm`; positional package syntax
stays unambiguous for scoped npm names. Human summaries use `npm:<name>` and
JSON keeps separate `source` and `package` fields. JSR should add a backend and
source validation behind the existing `PackageSource` and `PackageIdentity`
contracts. It should not change dependency tape identity or overload positional
package syntax. M69 does not implement JSR.

## M70+ handoff

- Add a first-class test dependency helper and execution contract before
  enabling `tspack add --dev`.
- Extend source projection with a separately proved `<Tools>` selection island,
  plus package executable metadata and ambiguity diagnostics, before enabling
  atomic `tspack add --tool`.
- Add JSR or other registries behind source-specific resolver backends while
  retaining source-qualified identity.
- If fully resolved remove dry-runs are required, let the manifest frontend
  evaluate candidate source bytes and let update consume that candidate IR;
  avoid temporary filesystem mutation.
- Compiler plurality (`tsc`, Copeland, PerryTS, ScriptC) likely needs
  target-scoped compiler acquisition and tool selection. It is separate from
  dependency editing and must preserve the manifest -> semantic IR -> backend
  direction.

## Stopping decision

Common native dependency edits have a coherent authoring-first path; package
targeting, source identity, no-op behavior, dry-run, JSON, provenance, and
authority guidance are explicit. The remaining dev/tool and multi-registry
work has concrete architecture prerequisites rather than M69 correctness gaps.

**M69 COMPLETE**
