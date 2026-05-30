# LLM Context Bundle Design

## Thesis

LLMs should receive runtime facts, not screenshots or inferred CSS guesses. A TSPack UI context bundle is a compact, structured, source-controlled, debuggable packet that captures what the browser actually rendered around a selected UI node.

The bundle is deliberately boring JSON. It is meant to be copied, tested with fixtures, attached to reviews, and inspected by humans before any future model integration exists.

## Non-goals

- No LLM provider integration.
- No auto-editing.
- No source mutation.
- No visual programming.
- No screenshot, OCR, or machine-vision pipeline.
- No framework-specific adapters in M40d.
- No prompt magic as the product contract.
- No automatic `tspack check` execution while building the bundle.
- No source-map lookup or heuristic dependency inference.

## Bundle inputs

The M40d prototype accepts these inputs:

- An inspect result from `tspack inspect` or the VS Code inspect tree state.
- A selected inspect node, found by object identity when it is still part of the inspect tree.
- An optional selected-node path, expressed as child indexes from the inspect root.
- A source hint from the selected inspect node, if present.
- A workspace root used only for source-hint validation and bounded excerpt reads.
- A validated source excerpt when the source hint is safe and the file exists inside the workspace.
- Optional TSPack diagnostics supplied by the caller; M40d does not run checks automatically.
- Optional package, run target, or browser target metadata in later callers.

## Bundle shape

The prototype JSON model is versioned and deterministic. It intentionally omits timestamps so fixture tests can compare serialized output.

```ts
type UIContextBundle = {
  version: 1;
  kind: "tspack.uiContext";
  createdAt?: never;
  workspace?: {
    rootName?: string;
  };
  selection: {
    nodeId?: string;
    path: number[];
    reason?: string;
  };
  runtime: {
    browser?: string;
    url?: string;
    viewport?: { width: number; height: number };
  };
  node: InspectNode;
  context: {
    ancestors: CompactInspectNode[];
    siblings: CompactInspectNode[];
    children: CompactInspectNode[];
    hitTests?: unknown[];
  };
  source?: {
    hint?: UISourceHint;
    validated: boolean;
    file?: string;
    line?: number;
    column?: number;
    excerpt?: {
      startLine: number;
      endLine: number;
      text: string;
    };
    validationError?: string;
  };
  diagnostics?: Array<{
    code: string;
    severity: string;
    message: string;
    file?: string;
    details?: string[];
  }>;
  constraints: string[];
};
```

`node` is the selected inspect node with its full runtime details. `CompactInspectNode` is used for surrounding context and includes only fields that help orient a model without exploding token size:

- `id`
- `tag`
- `role`
- `name`
- `text`
- `bounds`
- `visible`
- `focusable`
- `source.component`
- `source.symbol`

The bundle should answer these product questions:

- What did the browser render?
- Where is the selected node in viewport coordinates?
- What semantic role, accessible name, and text did the runtime expose?
- What ancestors, nearby siblings, and immediate children surround it?
- What source file/location probably produced it?
- What source excerpt is safe and relevant?
- What diagnostics already apply?
- What constraints should not be violated while suggesting changes?

## Trust model

Inspect node source hints are untrusted page data. A page can emit misleading or malicious `data-tspack-source` values. The bundle builder therefore treats source hints as navigation suggestions, not authority.

The M40d builder follows these rules:

- Source file access requires workspace validation.
- The builder never opens or reads paths outside the selected workspace root.
- Absolute paths are rejected.
- URL-like schemes such as `file:///...` and `https://...` are rejected.
- Parent traversal segments are rejected.
- Symlink escapes are rejected after realpath resolution.
- Missing files produce a validation error and no excerpt.
- Source excerpts are read-only context.
- The bundle is context, not authority; the source tree and tests remain authoritative for changes.

## Size budget

The bundle is compact by construction:

- The selected node is included in full.
- Ancestors are compact nodes from the inspect root to the selected node's parent.
- Siblings are compact nodes around the selected node, capped at five before and five after.
- Children are compact immediate children, capped at twenty.
- Compact names and text are truncated to two hundred characters plus an ellipsis.
- Source excerpts use an eight-line-before and twelve-line-after window around the hinted line.
- Source files with no line hint include only the first forty lines.
- Diagnostics are capped at twenty entries and are supplied by the caller.

Future versions may add relevance scoring for diagnostics and hit-test points, but M40d keeps the prototype deterministic and easy to review.

## Author/reviewer use

The intended workflow remains human-orchestrated:

1. A human inspects a real running UI and selects a node.
2. The IDE copies a context bundle describing the selected node, surrounding runtime tree, safe source excerpt, and diagnostics.
3. An author LLM can receive the bundle and propose a patch from observed runtime facts rather than broad source scraping or screenshots.
4. A reviewer LLM can receive the same bundle plus the human-authored patch and test output to critique the change.
5. A human decides what to apply, runs tests, and commits normal text diffs.

Native `assert.inspect.*` helpers can validate selected UI facts before or after an LLM-proposed change: for example, the same role/name/bounds/source/hit-test facts copied into a bundle can become explicit xTest assertions. This keeps bundle use grounded in executable checks without adding model calls to the test harness.

This separates runtime observation from code mutation. M40d provides the packet, not the agent.

## Future ladder

- M40d: design document, TypeScript model, pure builder prototype, fixture tests, and a low-risk VS Code copy command.
- M40e: native xTest inspect assertion helpers over collected inspect JSON.
- M40f: harden the VS Code **Copy LLM Context** command based on real use.
- Future: model-provider integration.
- Future: source-map and framework adapters.
- Future: visual overlay for observation and review.

## M40d prototype behavior

The VS Code extension prototype exposes `buildUiContextBundle(inspectResult, selectedNode, options)` as a pure async builder. The async part is limited to optional workspace-contained source excerpt reads.

The extension command **TSPack: Copy Selected Inspect Node LLM Context** copies pretty-printed JSON to the clipboard. It does not call any model, mutate source, open network connections, run `tspack check`, or generate prompts. If the selected node has a source hint, the command asks for a workspace root when needed so the existing M40c validation rules can protect excerpt reads.
