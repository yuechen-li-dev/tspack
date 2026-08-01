# TSPACK-BROWSER-RUNTIME-SOURCE-M0

## Canonical source and ownership

**TSPack owns the canonical browser runtime source.** Its discoverable entry
point is [`cmd/tspack/runtime/browser-v1/index.js`](../cmd/tspack/runtime/browser-v1/index.js).
It is an ordinary browser ESM module: runtime tests import that exact file in
Chromium, and future Vite development support can consume it as a normal
module.

Copeland owns component, layout, state, transition, attachment, capability,
adapter-selection, and artifact meaning. The runtime realizes those emitted
facts in a browser. Renderer adapters own their opaque DOM subtrees; an
application owns only its own bootstrap, including a React application root
where it has one.

**Go materializes and configures the runtime; it does not independently
reimplement browser lifecycle semantics.** `browser_materialization.go` embeds
the checked-in source to make output independent of its current working
directory, writes it unchanged as `@copeland/browser-v1`, validates artifact
transport boundaries, and writes deterministic imports, loaders, and host
HTML.

**Generated browser output is a projection, never a source-of-truth file.**

## Runtime contract

The generated-host public contract consists of the existing helper exports:

- `setText`, `onClick`, `dispatch`, `getMountElement`, `dispatchReact`,
  `copyText`, `getViewportWidth`, and `subscribeViewport`;
- attachment APIs `scheduleRendererAttachment`, `registerAttachmentPlans`,
  `attachRenderer`, `updateRenderer`, `detachRenderer`,
  `shutdownAttachmentPlans`, and `inspectAttachmentRuntime`;
- frame APIs `registerComponentFrames`, `dispatchComponentEvent`,
  `destroyComponentFrame`, `shutdownComponentFrames`,
  `inspectComponentFrame`, and `inspectComponentFrameTrace`.

`registerRendererAdapter` and `hasRendererAdapter` are the adapter API. An
adapter provides `mount`, `update`, and `unmount`; a compiler-emitted attachment
plan—not application code—selects it by ID. The built-in `CustomElement`
adapter remains the current direct execution bridge. Everything else in the
module, including maps, host observation, plan comparison, frame projection,
and diagnostic helpers, is internal runtime API.

The runtime consumes `attachments.json` schema v1 and optional compiler-emitted
`component-frames.js`. Attachment plans remain a versioned transport projection
of `HostAttachmentMir`; component frames deliberately remain an unversioned
executable transport in this milestone. Versioning that contract is the
immediate follow-up.

## Determinism and tests

Materialization copies the embedded source bytes unchanged. The Go
materialization test performs two materializations with identical inputs and
requires byte-identical host output; artifact metadata SHA-256 values are
derived from emitted bytes. The runtime source contains no timestamps,
absolute paths, random IDs, or environment-dependent ordering.

`cmd/tspack/runtime/browser-v1/runtime.test.mjs` is a focused Chromium test of
the source module itself. It covers missing-host contextual diagnostics,
Custom Element mount/update/unmount, host replacement recovery, child teardown,
adapter registry protection, component-frame event dispatch and child-frame
appearance/disappearance, destroyed-frame rejection, traces, and shutdown.
Go tests retain responsibility for materialization and artifact contract
validation; website/browser proofs retain DOM realization coverage.

## Compatibility sample inventory

The following checked-in sample runtime files are compatibility bridges, not
runtime authority: `copeland-website-m0`, `react-components-m1`,
`tsxml-react-m0`, `browser-dispatch`, `standalone-web-m0`,
`machina-layout-z-m0`, `machina-layout-table-m0`, and
`machina-table-derivation-m0`. They have fixture/manifests or historical proof
consumers and are retained unchanged in this milestone. No sample is silently
repointed, because their helper contracts are older and narrower than the
generated attachment/frame runtime.

## Vite readiness

There is no Vite migration here. Because the runtime is standard ESM source,
a future path may take Copeland artifacts, materialize the runtime and entry,
then let Vite serve or bundle them. Vite stays infrastructure: it does not own
or reinterpret semantic facts.
