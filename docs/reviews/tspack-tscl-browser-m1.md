# TSPACK-TSCL-BROWSER-M1 — browser materialization progression

## Closure path

TSPack now has an explicit target-level `javascriptRuntime` field. It defaults
to `node` for existing manifests; `browser` is an additional realization:

```tsx
<Targets rows={[{
  name: "browser",
  entry: "src/Main.ts",
  runtime: "dist/browser/main.js",
  javascriptRuntime: "browser",
  npmContracts: [{
    package: "nanoid",
    exports: [{ name: "nanoid", parameters: [], result: "string" }],
  }],
}]} />
```

TSPack resolves and locks npm in its existing update/sync path, supplies typed
package contracts to `tscl`, and invokes `tscl build` with
`javascriptRuntime: "browser"` and `javascriptProfile: "production"`.
`tscl` emits the Copeland local graph as browser ESM and returns its declared
entry rather than requiring TSPack to scan output directories.

For browser targets TSPack selects one of two realization modes. Native ESM
packages are copied into `dist/browser/packages/<deterministic-package-name>/`.
CommonJS-compatible package entries are transformed by the repository-local
`esbuild@0.21.5` binary into one production browser ESM artifact per requested
entry. The transformer receives only selected npm package entries; it never
receives authored Copeland source. TSPack creates a generated
`@copeland/browser-v1` host module, a generated HTML host, `import-map.json`,
and `browser-materialization.json`.

The resolver selects exports in this order:

1. requested `exports` subpath with `browser`;
2. requested `exports` subpath with `import`;
3. requested `exports` subpath with `default`;
4. `browser`, `module`, then `main` fallback.

It rejects unsafe output paths. CommonJS façades use transformed ESM when
esbuild can browser-bundle them; transformer failures remain package-specific
browser-realization errors. Copeland itself still never scans `node_modules`.

## Representative proof

`fixtures/tscl-browser-m1` is a TSPack-managed multi-module Copeland project.
It locks `nanoid@5.1.16`; TSPack selects its actual `exports["."].browser`
entry, `index.browser.js`, and emits this import map:

```json
{"imports":{"@copeland/browser-v1":"./packages/copeland-browser-v1/index.js","nanoid":"./packages/nanoid/index.browser.js"}}
```

The emitted module graph contains `src/Main.js`, `src/State.js`, and
`src/View.js`. `src/Main.js` now emits `const generatedId = nanoid();`; the
returned native-ESM package value is stored in immutable Copeland state and
rendered. A fresh Chromium context showed
`Browser package call: 9kd4Xc0rM4k7RA6UmXZg6; Count: 0`, then the same
21-character value with `Count: 1` after a real button click. Its error-level
console log was empty.

Commands used:

```text
go run ./cmd/tspack update --root fixtures/tscl-browser-m1
go run ./cmd/tspack sync --root fixtures/tscl-browser-m1
go run ./cmd/tspack build --root fixtures/tscl-browser-m1 browser
python -m http.server 4173 --directory fixtures/tscl-browser-m1/dist/browser
```

The direct ESM graph and interaction load through static HTTP; no Node process
executes application JavaScript. A failed `tscl` build removes the published
runtime entry (`dist/browser/main.js`) so the preceding artifact cannot remain
the current runnable entry.

## React preflight

The existing TSPack-locked React project at `.tmp/m58a-hello-react-fixed2`
resolves `react@19.2.7` and `react-dom@19.2.7`. The selected root entry for
`react` is `./index.js` through `default`; the selected entry for
`react-dom/client` is `./client.js` through `default`. Both are CommonJS
façades: they branch on `process.env.NODE_ENV` and call `require` into `cjs/`.
React production and development are selected inside those façades, not through
a browser ESM export condition.

TSPack transforms both entries with repository-local `esbuild@0.21.5`, defining
`process.env.NODE_ENV` as the literal `"production"`. The `react-dom/client`
transform externalizes `react` and supplies its CommonJS façade through a
generated lexical `require` binding that returns the import-mapped React ESM
module. This keeps ReactDOM/client bound to the same `react` import-map
singleton rather than bundling a second React copy.

The generated preflight imports `react` and `react-dom/client`, validates
`createElement` and `createRoot`, and creates/unmounts a detached root. It does
not render a React tree or add Copeland React semantics. Chromium completed the
preflight with no error-level console entries.

## Additional work performed

- Added direct synchronous npm-call binding/MIR/JavaScript emission for declared
  named exports without a remote error. Existing async/TSON npm transport stays
  in place for contracts that declare a remote error.
- Added browser target parsing, result target metadata, and a browser ESM
  launcher in `tscl`; Node launch/output remains unchanged.
- Added manifest target npm contract rows and browser-aware TSPack request and
  fingerprint construction; browser and Node cache keys differ.
- Added bounded native-copy and transformed-ESM package materialization,
  import-map/materialization artifacts, and focused tests.
- Added the browser fixture and browser-specific compiler integration test.

## Current non-goals

No general application bundler, arbitrary CommonJS conversion, React mounting,
TS-XML profile, dynamic imports, Node polyfills, arbitrary asset pipeline,
HMR/watch, SSR, hydration, or source maps is claimed.

## Status

The browser realization closure is complete for the bounded M1 law: native ESM
copying and selected CommonJS façade transformation both execute in Chromium.
React-specific Copeland contracts and TS-XML semantics remain deferred to
CTS-TSXML-REACT-M0.
