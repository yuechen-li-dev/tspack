# Runtime-Grounded IDE / Inspect Closeout

The runtime-grounded IDE / inspect track closes in **Success**. The current implementation gives TSPack a structured browser/runtime observation layer, a VS Code proof-of-concept consumer, native xTest helpers, source-hint navigation, and an LLM-ready context bundle shape without changing source from runtime data or introducing visual-programming behavior.

## Original motivation

Modern frontend source no longer directly represents final UI truth. Framework conditionals, generated markup, CSS cascade, layout engines, accessibility computation, visibility, focusability, scrolling, and hit testing are resolved by the browser or host runtime.

Browser-computed layout and accessibility facts matter because they are the facts users, tests, IDEs, and reviewers need when diagnosing UI behavior. LLMs should not infer final layout from CSS/source soup when the runtime can report the rendered structure directly.

Figma and other design artifacts are useful inputs, but they are not runtime application truth. Cursor-style chat in an editor is also not enough by itself: a model sitting beside source still lacks structured facts about the running UI unless the tooling supplies them.

TSPack inspect bridges text source and runtime truth by extracting browser-computed facts as structured data that IDEs, xTests, and future LLM workflows can consume.

## Remediation / implementation summary

| Need / finding | Milestone | Fix | Status |
| --- | --- | --- | --- |
| Inspect selector and point options were not reliably reaching the analyzer; CDP/Electron target discovery needed a stronger fallback; WebKit aliases were missing. | M39a | Routed selector/point arguments into analyzer invocation, added `Target.getTargets` fallback for CDP/Electron hosts, and accepted Playwright WebKit backend aliases. | Done |
| IDE workflow needed a concrete consumer of inspect JSON. | M39b | Added the VS Code extension POC under `extensions/tspack-vscode`; it shells out to `tspack inspect`, lists CDP targets, renders an inspect tree, shows selected-node details, and copies selected-node JSON. | Done |
| Native xTest needed direct observation helpers that reuse inspect without counting observation as assertion. | M40a | Added `inspect.url(...)`, `inspect.cdp(...)`, `inspect.target(...)`, and `inspect.cdpTarget(...)` helpers that return plain inspect JSON and remain observation-only. | Done |
| Runtime nodes needed optional source/context hints without treating page data as trusted filesystem authority. | M40b | Documented source mapping strategy and parsed optional `data-tspack-source`, `data-tspack-component`, and `data-tspack-symbol` hints as untrusted navigation/context metadata. | Done |
| IDE users needed safe source navigation from selected inspect nodes. | M40c | Added read-only reveal-source in the VS Code extension with workspace-contained path validation and rejection of absolute paths, traversal, URL-like schemes, missing files, and symlink escapes. | Done |
| Future LLM workflows needed a deterministic context packet instead of broad source scraping or provider calls. | M40d | Added an LLM context bundle design, a pure bundle builder, and **TSPack: Copy Selected Inspect Node LLM Context** in the extension. | Done |
| xTest needed assertions over collected runtime facts. | M40e | Added `assert.inspect.exists`, `visible`, `hidden`, `role`, `name`, `boundsWithin`, `hitIncludes`, and `source`; helpers count as assertions and require reasons. | Done |

## Current inspect model

`tspack inspect` can observe a URL, an explicit CDP endpoint, a run target, or an existing CDP/Electron/VS Code target. Playwright-backed URL inspection supports Chromium and WebKit backend names, including short aliases. CDP target discovery lists regular `/json` targets and falls back to `Target.getTargets` when Electron-style hosts expose renderer targets only through the browser CDP session.

The analyzer reports structured runtime data rather than pixels. The current model includes DOM/layout/accessibility-ish structure, browser-computed bounds, visibility, focusability, selected styles, hit-test results, diagnostics, and optional source hints. It does not use screenshots, OCR, or machine vision.

## Current VS Code extension model

The extension is a proof of concept that shells out to the TSPack CLI instead of reimplementing inspection. It supports CDP target listing, selected target inspection, inspect tree rendering, selected-node details, selected-node JSON copy, safe reveal-source from validated source hints, and copying a selected-node LLM context bundle.

The extension is read-only. It does not mutate source, perform visual editing, add drag handles, call an LLM provider, open network connections for model use, or become a Code-OSS fork.

## Current xTest inspect model

Native xTest can call `inspect.url(...)` and `inspect.cdp(...)` to collect the same structured inspect JSON used by the CLI. `inspect.target(...)` and `inspect.cdpTarget(...)` provide target/run-oriented convenience wrappers.

Inspect-only calls are observations and do not satisfy the native no-assertion rule. Tests make claims with `assert.inspect.*` helpers, normal assertions, or snapshots. `assert.inspect.*` validates already-collected facts such as existence, visibility, hidden state, role, accessible name, bounds, source hints, and hit-test membership. Stable selected subtrees can be captured with `expect.snapshotJson(...)`. Browser-backed tests skip cleanly when the required browser runtime is unavailable.

## LLM context model

The current LLM context bundle is a deterministic JSON packet for the selected inspect node. It includes the selected node, compact surrounding context, runtime metadata, a workspace-validated source excerpt when safe, a diagnostics slot, and explicit constraints.

There is no provider integration yet. The bundle copy command does not call OpenAI, Claude, or any other model, and it does not mutate source.

## Current golden workflow

```sh
tspack run dev
```

Then inspect the running app with the supported path for the project, for example:

```sh
tspack inspect --cdp http://127.0.0.1:9229 --list-targets
tspack inspect --cdp http://127.0.0.1:9229 --target 0 --json
```

In VS Code:

1. Run **TSPack: Inspect CDP Targets**.
2. Select a node in the TSPack Inspect tree.
3. Run **TSPack: Reveal Source for Selected Inspect Node** when the node has a safe source hint.
4. Run **TSPack: Copy Selected Inspect Node LLM Context** to copy a deterministic runtime/source context bundle.

In native xTest:

```tsx
const ui = await inspect.url("http://127.0.0.1:5173", {
  selector: "main",
});

const save = inspect.findByRole(ui.root, "button", "Save");

assert.inspect.visible(save, "Save button should be visible");
expect.snapshotJson(ui.root, "home-main-landmark").because(
  "main landmark runtime structure should remain stable",
);
```

## Explicit non-goals / deferred work

- No visual programming as source of truth.
- No WYSIWYG mutation.
- No source mutation from the extension.
- No LLM provider integration yet.
- No Code-OSS fork yet.
- No framework adapters yet.
- No React internals dependency.
- No source-map or bundler integration yet.
- No screenshot, OCR, or machine vision path.
- No Storybook clone.
- No browser automation actions in inspect assertions.
- No retry or auto-wait semantics in inspect assertions.

## Future ladder

- Source hint transforms and instrumentation.
- Framework adapters.
- LLM author/reviewer bundle workflows.
- Inspect assertion libraries.
- Visual overlay for observation and review.
- Optional Code-OSS fork if extension APIs become limiting.
