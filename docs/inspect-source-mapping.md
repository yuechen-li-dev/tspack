# Inspect Source Mapping Design

## Problem

Runtime inspect knows what the browser rendered: DOM structure, accessibility role/name, text, computed layout bounds, visibility, focusability, styles, hit-test results, and the inspected target. IDE, test, and future assistant workflows also need a best-effort way to connect a selected runtime node back to likely source.

CSS selectors, DOM tags, and rendered text are not enough to identify source ownership reliably. A single component can render many nodes, build tools can rewrite code, framework runtimes differ, and production bundles often erase the information humans want. TSPack should therefore treat source mapping as a staged set of hints and probes, not as a perfect framework-agnostic guarantee.

## Non-goals

M40b does not implement or require:

- perfect framework-agnostic source mapping
- visual editing or drag handles
- source mutation
- framework adapters or compiler plugins
- React internals or React DevTools protocol integration
- source-map generation or build-pipeline integration
- screenshot, OCR, or machine-vision inference
- Storybook integration
- LLM context bundle generation

Text remains the source of truth. Browser/runtime state remains the layout truth. Source mapping is an optional bridge between those facts.

## Strategy ladder

1. **Source hints**
   - Read optional `data-tspack-source`, `data-tspack-component`, and `data-tspack-symbol` attributes from rendered DOM nodes.
   - Deterministic, framework-agnostic, and useful immediately in tests, examples, and hand-instrumented fixtures.
   - Does not require TSPack to understand a framework or build tool.

2. **Source hint transform**
   - Future dev-only transforms can inject the same hint attributes during JSX/template compilation or test fixture rendering.
   - The runtime analyzer contract stays the same; transforms are only producers of hints.

3. **Heuristic source search**
   - Future fallback can search source files by rendered text, class names, roles, test IDs, tag patterns, and component/symbol names.
   - This is best-effort only and must return candidates with confidence/evidence rather than authoritative locations.

4. **Framework adapters**
   - Later React, Vue, Svelte, Solid, or other adapters can provide richer framework-native ownership data.
   - Adapters should feed the same source model rather than replacing inspect output with framework-specific shapes.

5. **IDE reveal**
   - M40c adds a VS Code command that validates project-relative source hints and opens an existing file/line/column when the path stays inside the selected workspace.
   - Reveal-source consumes hints as navigation suggestions, not as trusted filesystem paths.

## Source hint contract

The M40b prototype defines a small, optional, framework-agnostic source hint contract. Pages may render any of these attributes:

```html
<button
  data-tspack-source="src/components/Button.tsx:42:7"
  data-tspack-component="Button"
  data-tspack-symbol="Button.Primary"
>
  Save
</button>
```

### Attributes

- `data-tspack-source`
  - Accepted forms:
    - `<file>`
    - `<file>:<line>`
    - `<file>:<line>:<column>`
  - `line` and `column` are positive integers when present.
  - The analyzer reports the value only; it does not resolve, open, normalize against the filesystem, or validate that the file exists.
- `data-tspack-component`
  - Optional human/framework component name such as `Button`.
- `data-tspack-symbol`
  - Optional finer-grained symbol or variant such as `Button.Primary`.

TSPack intentionally does not claim ownership of generic `data-testid` or framework-specific attributes in this contract.

### JSON output shape

When a node has at least one source hint attribute, inspect may include `source` on that node:

```ts
type UISourceHint = {
  raw?: string;
  file?: string;
  line?: number;
  column?: number;
  component?: string;
  symbol?: string;
  parseError?: string;
};
```

Example inspect node fragment:

```json
{
  "tag": "button",
  "role": "button",
  "name": "Save",
  "source": {
    "raw": "src/components/Button.tsx:42:7",
    "file": "src/components/Button.tsx",
    "line": 42,
    "column": 7,
    "component": "Button",
    "symbol": "Button.Primary"
  }
}
```

If `data-tspack-source` is malformed, inspect does not fail. The node preserves `source.raw` and includes `source.parseError`; component and symbol hints are still reported when present. A node without source hint attributes omits `source`.

## Trust and security

Source hints are untrusted page data. TSPack must treat them as hints for display, assertions, snapshots, and navigation. They are not authority to read files or mutate source.

The M40c VS Code reveal command is read-only. Before opening a file, it rejects absolute paths, URL-like schemes, and any `..` parent traversal segment after normalizing backslashes to slashes. It resolves the remaining relative path against the selected workspace root, requires the file to exist, resolves real paths for both workspace root and target, and rejects symlinks that escape the workspace. It never creates files, edits files, or uses `component` or `symbol` values for filesystem resolution.

`data-tspack-source` line and column values are one-based. VS Code editor positions are zero-based, so reveal converts `42:7` to line `41`, column `6`. Missing line or column values reveal the top of the file, and out-of-range editor positions are clamped to a valid document position.

## LLM context

Source hints let future tools pass an LLM a compact evidence bundle:

- runtime node structure
- browser-computed bounds and visibility
- accessibility role/name and text
- probable source file/line/column/component/symbol

That reduces guessing without turning runtime inspection into source of truth. M40b only defines and prototypes the hint field; it does not build an LLM context bundle.

## Heuristic search probe notes

Heuristic source search remains design-only for M40b. Useful future probes include:

- `findByText`-style searches for exact rendered text in TSX/templates
- class name searches for stable CSS module or utility class tokens
- test ID searches when a project already uses `data-testid` or equivalent conventions
- component-name searches seeded by `data-tspack-component` or framework adapter metadata
- role/name searches that produce candidate source snippets with low confidence unless corroborated

Any heuristic result should include evidence, confidence, and ambiguity rather than claiming a single source owner.

## Future milestones

- **M40c**: add a safe VS Code reveal-source command for workspace-contained source hints.
- **M40d**: refine source hint analyzer support and add more fixture coverage if the source hint prototype proves too narrow.
- **M40e**: define an explicit LLM context bundle shape that can include source hints.
- **M40f**: expand inspect UI assertions around source metadata and runtime facts.
