# TSPACK-COMPONENT-FRAMES-CONTRACT-V1

## Finding and ownership

The previous `component-frames.js` imported `registerComponentFrames` and
installed object literals containing executable `transition` and `project`
closures. Its data carried stable component/frame identities, initial state,
zero-payload events, attachment IDs, branch child identities, and Custom
Element payloads. The closures implemented constant or state-match transitions
and label/child projection.

V1 replaces that artifact with an inert default-exported envelope:

```js
export default {
  schemaVersion: 1,
  projectId: "copeland",
  frameDefinitions: [{
    frameDefinitionId: "...",
    componentDefinitionId: "...",
    stateIdentity: "...",
    attachmentIds: ["..."],
    events: [{ eventId: "...", name: "...", payloadContract: "void", transition: { kind: "constant", nextState: "..." } }],
    presentationBranches: [{ branchId: "...", statePattern: "...", childFrames: [] }],
    source: { path: "", line: 0, column: 0 },
  }],
  frameInstances: [{ componentInstanceId: "...", frameDefinitionId: "...", parentComponentInstanceId: null, initialState: "..." }],
};
```

**Copeland emits versioned component-frame meaning. TSPack’s browser runtime
executes it.** The artifact carries compiler projections but does not own
component semantics. **Runtime execution is fixed and versioned; generated
projects do not install arbitrary frame schedulers.**

## Fixed executor and APIs

`@copeland/browser-v1` exposes `registerComponentFrameEnvelope` as the
generated-bootstrap API. It validates V1, creates fixed transition/projection
executors inside the runtime, then delegates to the existing frame lifecycle:
registration, typed dispatch, immutable state replacement, branch child
reconciliation, attachment replacement, traces, and deepest-first cleanup.

`dispatchComponentEvent`, `destroyComponentFrame`, `shutdownComponentFrames`,
and inspection APIs remain application-facing compatibility APIs. Envelope
validation and executor helpers are internal runtime API. The legacy
`registerComponentFrames` API remains only as a compatibility bridge for an
unversioned module; new compiler output does not call it.

The supported emitted subset is intentionally bounded: string or nullary-enum
state, zero-payload events, constant or state-match transitions, and Custom
Element attachment projections. Effects, general expressions, and arbitrary
renderer behavior remain outside browser V1.

## Validation and compatibility

Materialization recognizes only either a default-exported V1 envelope with
`schemaVersion: 1`, or the explicit legacy `registerComponentFrames` side
effect module. Other shapes are rejected. The browser runtime validates V1
top-level fields, duplicate definition/instance/event/branch identities,
definition references, transition vocabulary, child descriptors, and schema
version with contextual diagnostics.

The generated loader imports the artifact once. A default export is passed to
the V1 executor; a module with no default is treated as the documented legacy
side-effect bridge. Legacy modules are not interpreted as V1. The bridge is
deprecated and is retained only for existing browser-v1 fixtures; remove it in
the next browser-v2 compatibility decision after fixtures migrate.

## Browser-v2 migration policy

`browser-v1` accepts V1 envelopes as the canonical path and accepts a legacy
side-effect `registerComponentFrames` module only through its compatibility
loader. A valid legacy module produces the runtime trace
`LegacyFrameContractLoaded`, including `component-frames.js`, with migration
guidance but no production console warning. A V1 envelope produces no legacy
trace. A module that mixes both paths, or provides neither, is rejected with a
contextual V1 diagnostic.

`browser-v2` will accept only explicitly supported versioned envelope schemas;
it will not expose or load the side-effect registration bridge. Browser-v1
remains the host contract for old artifacts. The removal condition is not a
calendar date: all maintained samples/templates must emit V1, the explicit
legacy fixture must be the only bridge consumer, and one release line must
have shipped the deprecation trace. A future frame schema requires a new
explicit runtime support decision rather than fallback guessing.

### Migration guide

1. Identify legacy output by an import of `registerComponentFrames` or a
   `LegacyFrameContractLoaded` trace.
2. Regenerate a compiler-driven project with the current Copeland CLI; it
   emits the V1 default envelope automatically.
3. Convert a hand-authored fixture to `export default { schemaVersion: 1,
   projectId, frameDefinitions, frameInstances }`; do not wrap its old
   closures or execute it to capture state.
4. Build through TSPack, then run its browser proof. Browser-v2 will reject
   the old side-effect form.

For the maintained-consumer inventory, deprecation trace, and browser-v2
removal condition, see [browser-v2-migration-m0.md](browser-v2-migration-m0.md).

## Determinism and equivalence

Copeland emits definitions and instances in stable component identity order,
events by name, and branches/children in bound presentation order. The envelope
uses only semantic IDs and project-relative/empty provenance fields—never DOM
nodes, roots, absolute paths, random IDs, or timestamps. TSPack hashes the
materialized artifact and copies the canonical runtime unchanged.

The envelope maps compiler facts directly: component state identity and initial
state become a definition/instance; event and transition table facts become
`events`; presentation branch/child facts become `presentationBranches`; and
attachment identity is retained in definition and child descriptors. Runtime
state remains runtime-only.
