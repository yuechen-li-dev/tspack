# TSPACK-BROWSER-V2-MIGRATION-M0

## Legacy consumer inventory

| Consumer | Classification | Executed path | Prior semantics | Action |
| --- | --- | --- | --- | --- |
| `cmd/tspack/runtime/browser-v1/index.js` | Active compatibility implementation | generated browser-v1 host | accepts legacy frame array registration | retained, deprecated API only |
| `cmd/tspack/browser_materialization.go` loader | Active compatibility implementation | every materialized browser frame artifact | selects V1 default export or legacy side effect | retains one explicit bridge and trace |
| `fixtures/browser-v1-legacy-component-frames/component-frames.js` | Active compatibility fixture | focused materialization/runtime coverage | empty legacy registration module | retained as the sole intentional legacy artifact |
| `cmd/tspack/runtime/browser-v1/runtime.test.mjs` | Active compatibility test | Chromium source-runtime test | legacy registration plus V1 execution | retained as proof, not an authoring example |
| `samples/copeland-ts/copeland-website-m0/browser-proof.mjs` | Active product/sample proof | Desktop/Tablet/Mobile browser proof | hand-authored frame registration closures | migrated to hand-authored V1 test envelopes |
| `samples/copeland-ts/standalone-web-m0/frontend/generated/component-frames.js` | Historical but still referenced fixture | standalone generated frontend input | empty legacy registration | migrated to an empty V1 envelope |
| Copeland `ComponentFrameArtifactEmitter` | Active compiler generator | all new browser builds | legacy registration module | already emits V1 only |
| Copeland templates and supported scenario sources | Active generator/sample paths | normal build/scenario paths | no legacy registration source found | guarded V1-only |

No consumer was classified dead based only on age. The maintained-project guard
checks the compiler emitter, flagship website proof, and standalone historical
fixture; it explicitly permits only the dedicated TSPack legacy fixture.

## Browser-v1 compatibility law

Browser-v1 accepts a default-exported V1 frame envelope as the normal path. It
also accepts an unversioned side-effect `registerComponentFrames` module only
when it registers during the generated frame loader. Valid legacy loading adds
the `LegacyFrameContractLoaded` runtime trace with `component-frames.js` and
migration guidance; it is not a failure and does not write production console
noise. V1 loading adds no deprecation trace.

The loader rejects a mixed default-envelope plus legacy registration module
with `COPE-COMPONENT-STATE-V1-1020`, and a module using neither recognized path
with `COPE-COMPONENT-STATE-V1-1021`. Duplicate frame identity continues to use
the runtime’s existing contextual duplicate-frame diagnostic.

## Browser-v2 policy and migration

Browser-v2 will have a distinct browser host contract/package identity and
will load only explicitly supported versioned envelopes. It will not export or
execute the side-effect registration bridge; unknown and unversioned frame
modules are rejected. Browser-v1 remains available to execute old artifacts.

Remove the bridge only after every maintained sample/template emits V1, the
dedicated fixture is the only intentional bridge consumer, and one release
line has shipped the browser-v1 deprecation trace. Future schemas require an
explicit runtime support decision; no version fallback is inferred.

To migrate, regenerate compiler-driven projects with current Copeland. For a
hand-authored fixture, replace the import/call with an inert default envelope
containing `schemaVersion: 1`, `projectId`, `frameDefinitions`, and
`frameInstances`; do not execute the old module to capture runtime state. Build
through TSPack and run the applicable browser proof. Browser-v2 will reject the
old form.
