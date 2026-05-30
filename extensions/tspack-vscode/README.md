# TSPack Inspect VS Code Extension POC

This is the M39b proof-of-concept VS Code extension for viewing `tspack inspect` CDP output inside VS Code.

## Local development

```sh
npm install
npm run compile
npm test
```

## Runtime workflow

1. Start VS Code, Chromium, or another Chromium/Electron host with a remote debugging port, for example:

   ```sh
   code --remote-debugging-port=9229
   ```

2. Configure these VS Code settings when defaults are not correct:
   - `tspack.inspect.cdpEndpoint` (default `http://127.0.0.1:9229`)
   - `tspack.inspect.tspackPath` (default `tspack`)
   - `tspack.inspect.targetIndex` (default `0`)

3. Run **TSPack: Inspect CDP Targets** to list targets and inspect one.
4. Use **TSPack: Refresh Inspect Tree** to re-run inspect for the configured target.
5. Select a tree node to print JSON details in the **TSPack Inspect** output channel.
6. Use **TSPack: Copy Selected Inspect Node JSON** to copy the current node payload.
7. Use **TSPack: Copy Selected Inspect Node LLM Context** to copy a deterministic JSON context bundle for the selected node. The bundle includes runtime facts, compact surrounding tree context, constraints, and a workspace-validated source excerpt when safe. It does not call a model.
8. If the selected node has a parsed source hint, use **TSPack: Reveal Source for Selected Inspect Node** to open the existing workspace file at the hinted location.

The extension intentionally shells out to `tspack inspect` and does not reimplement CDP inspection.


## Reveal source from inspect hints

Rendered DOM may include optional TSPack source hints:

```html
<button data-tspack-source="src/components/Button.tsx:42:7">
  Save
</button>
```

The source format is `<file>`, `<file>:<line>`, or `<file>:<line>:<column>`. Line and column values are one-based in the DOM hint; the extension converts them to VS Code's zero-based editor positions. If line or column is absent, the file opens at the top.

Source hints are untrusted page data. **TSPack: Reveal Source for Selected Inspect Node** is read-only and refuses to open hints that are absolute paths, URL-like values such as `file:///...` or `http://...`, contain parent traversal (`..`), point outside the selected workspace folder, or resolve through a symlink outside the workspace. The hinted file must already exist; the extension does not create or modify files.

If no workspace is open, the command warns that a workspace folder is required. If multiple workspace folders are open, the command asks which folder should be used as the root for that reveal operation. Nodes without source hints, malformed raw hints, unsafe paths, and missing files produce warning messages instead of filesystem mutation.

## Copy selected node LLM context

**TSPack: Copy Selected Inspect Node LLM Context** copies a pretty-printed `tspack.uiContext` JSON bundle to the clipboard. The command is read-only: it does not call OpenAI, Claude, or any other model; it does not mutate source; it does not open network connections; and it does not run `tspack check` automatically.

When the selected node has a source hint, the command validates the hinted file against a selected workspace folder before reading a bounded excerpt. The same safety model as reveal-source applies: absolute paths, parent traversal, URL-like schemes, missing files, and symlink escapes are rejected and reported in the bundle as validation errors instead of excerpts.
