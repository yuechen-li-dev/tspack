# Runtime-Grounded IDE Vision

## Thesis

Modern AI editors are mostly text editors with chat beside them. TSPack's direction is runtime-grounded development: combine source truth, browser/runtime truth, structured diagnostics, and LLM-ready context so tools can reason from facts the runtime actually computed.

The source tree remains the durable system of record. The browser or application runtime remains authoritative for layout, accessibility, focus, visibility, hit-testing, and rendered structure. `tspack inspect` is the bridge that extracts those runtime facts into structured data that IDEs, tests, and future assistants can consume without guessing from CSS and source fragments alone.

## Non-goals

- Not visual programming as the source of truth.
- Not screenshot/OCR/machine-vision inference.
- Not Figma handoff as application truth.
- Not a Code-OSS fork as the first step.
- Not a framework-specific visual builder.
- Not hidden canvas state that bypasses reviewable text diffs.

## Core principle

Text remains source of truth. The browser/runtime remains layout truth. TSPack bridges them with structured inspect data.

A runtime-grounded IDE should preserve normal code review and source-control workflows while adding an observation layer that can answer concrete UI questions:

- Which node is actually rendered?
- What role, name, text, bounds, and styles did the runtime compute?
- Is the node visible and focusable?
- What target, viewport, diagnostics, and hit-test facts explain the current UI state?

## Current proof

- `tspack run` starts declared runtime targets.
- `tspack inspect` extracts DOM, layout, accessibility, style, visibility, and hit-test structure.
- CDP mode can inspect browser, Electron, and VS Code-family targets when an explicit remote-debugging endpoint is provided.
- The M39b VS Code extension proof of concept wraps `tspack inspect` output in an IDE panel without reimplementing CDP inspection.

## Future capabilities

- Click a UI node and inspect its structural runtime facts.
- Map an inspected node back to source files and framework/component locations.
- Build an LLM context bundle containing the selected node, bounds, role/name/text, computed styles, source file, and diagnostics.
- Add xTest UI assertions for role, name, visibility, bounds, and hit-test behavior.
- Add a visual overlay or design-surface observer that remains backed by text diffs.
- Consider a Code-OSS fork only if extension APIs become the limiting factor after the runtime-grounded workflow has proven useful.

## Why this is not Cursor

The goal is not merely an LLM chat panel next to source files. Runtime-grounded development gives author, reviewer, observer, test harness, and future assistant workflows the same structured facts produced by the running application.

That matters because layout and accessibility are emergent runtime properties. Asking a model to infer them from CSS soup, framework conditionals, and partial source context is brittle. Asking the runtime for structured facts is a stronger foundation.

## Why this is not Figma

Figma is a design artifact, not runtime truth. The rendered browser or Electron state is authoritative for actual web UI behavior, including computed styles, accessibility names, visibility, focusability, and bounds.

Future editing workflows should still become text diffs. The IDE can observe and explain rendered state, but it should not replace reviewable source with hidden canvas state.

## Architecture ladder

### M39b: VS Code extension CDP inspect proof of concept

- Configure a CDP endpoint and TSPack CLI path.
- List CDP targets through `tspack inspect --cdp <endpoint> --list-targets --json`.
- Inspect a selected target through `tspack inspect --cdp <endpoint> --target <index> --json`.
- Render the inspect JSON tree and selected node details in VS Code.
- Copy selected node JSON for bug reports, tests, or future context bundles.

### Future: source mapping

Connect inspected nodes to source locations without making runtime inspection framework-specific.

### Future: LLM context bundles

Package selected runtime facts, source mappings, diagnostics, and test intent into explicit context instead of relying on broad source scraping.

### Future: xTest inspect assertions

Use inspect facts for test assertions such as role/name, visibility, bounds, and hit-test order.

### Future: visual overlay

Add overlays for observation and review while keeping source text and diffs authoritative.

### Future: optional Code-OSS fork

Only consider a fork after the extension path proves the workflow and hits concrete API limits.
