# Runtime-Grounded IDE Vision

## Thesis

Modern AI editors are mostly text editors with chat beside them. TSPack's direction is runtime-grounded development: combine source truth, browser/runtime truth, structured diagnostics, and LLM-ready context so tools can reason from facts the runtime actually computed.

The source tree remains the durable system of record. The browser or application runtime remains authoritative for layout, accessibility, focus, visibility, hit-testing, and rendered structure. `tspack inspect` is the bridge that extracts those runtime facts into structured data that IDEs, tests, and future assistants can consume without guessing from CSS and source fragments alone.

See [Runtime-Grounded IDE / Inspect Closeout](claude-fooding-runtime-grounded-ide.md) for the M39a through M40f closeout state and release-gate posture.

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
- The M39b/M40c/M40d VS Code extension proof of concept wraps `tspack inspect` output in an IDE panel, displays source hints, can reveal safe workspace-contained source locations, and can copy a deterministic selected-node LLM context bundle without reimplementing CDP inspection, calling a model, or mutating source.

## Future capabilities

- Click a UI node and inspect its structural runtime facts.
- Map an inspected node back to probable source files and framework/component locations through optional source hints first.
- Build an LLM context bundle containing the selected node, bounds, role/name/text, computed styles, source file, and diagnostics.
- Expand xTest inspect assertion libraries around role, name, visibility, bounds, source hints, and hit-test behavior.
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

### M40b: inspect source mapping design probe

Connect inspected nodes to probable source locations without making runtime inspection framework-specific. The staged design starts with optional `data-tspack-source`, `data-tspack-component`, and `data-tspack-symbol` hints, then leaves transforms, heuristics, and framework adapters for later milestones. See [Inspect Source Mapping Design](inspect-source-mapping.md).

### M40c: safe VS Code reveal-source

The VS Code extension adds **TSPack: Reveal Source for Selected Inspect Node**. The command treats source hints as untrusted navigation suggestions: it opens only existing workspace-contained files, rejects absolute paths, parent traversal, URL-like schemes, and symlink escapes, and never creates or mutates files. Line and column hints use the one-based `data-tspack-source` contract and are converted to VS Code's zero-based editor positions.

### M40d: LLM context bundle copy

Package selected runtime facts, source mappings, diagnostics, and test intent into explicit context instead of relying on broad source scraping. The current extension command copies a deterministic selected-node bundle; provider integration, prompt orchestration, and source mutation remain deferred.

### Native xTest inspect assertions

Use `assert.inspect.*` as the validation layer over inspect facts. The helpers assert role/name, visibility, bounds, hit-test membership, and source-hint fields from already-collected inspect JSON. They count as native xTest assertions but do not perform browser actions, retries, re-inspection, source mutation, screenshot/OCR work, or IDE behavior.

### Future: visual overlay

Add overlays for observation and review while keeping source text and diffs authoritative.

### Future: optional Code-OSS fork

Only consider a fork after the extension path proves the workflow and hits concrete API limits.

## xTest inspect helper in the runtime-grounded loop

Native xTest can now participate directly in the runtime-grounded workflow:

1. **Observe** with `inspect.url` or `inspect.cdp`, reusing the same backend as `tspack inspect`.
2. **Assert** semantic structure with `assert.inspect.*`, such as roles, names, visibility, usable bounds, source hints, and selected hit-test results.
3. **Snapshot** stable subtrees with `expect.snapshotJson` when structural regressions should be reviewed.
4. **Provide context later** by handing exact inspect JSON to higher-level tools instead of guessing from CSS or source.

This step is intentionally limited to observation plus test assertions. Source hints may now appear in inspect JSON when a page provides them, but the helper does not add visual editing handles, source mutation, screenshots, OCR, reveal-source behavior, or LLM context bundles.

## M40d: LLM context bundle layer

The next runtime-grounded layer is an LLM context bundle: a deterministic JSON packet for a selected inspect node. The bundle combines the selected runtime node, compact ancestors/siblings/children, browser-computed role/name/text/bounds/style facts, optional hit-test facts, a workspace-validated source excerpt, diagnostics supplied by the caller, and stable constraints that remind tools not to overtrust hints or infer dependencies.

This layer is not LLM integration. It exists so author and reviewer workflows can share the same observed runtime facts while humans continue to orchestrate source edits, tests, and review. An author LLM can use the bundle to propose a patch; a reviewer LLM can later use the bundle plus a patch and test output to critique that patch. In both cases the source tree remains the source of truth, the browser remains layout truth, and the bundle is read-only context.

See [LLM Context Bundle Design](llm-context-bundle.md) for the M40d contract and prototype behavior.
