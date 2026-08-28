# M73 inspect instrumentation architecture

`InspectSourceInstrumentation` owns one semantic operation: given source text
and an authored source path, add bounded inspect provenance for a development or
test build. It is independent of Vite. The Vite adapter is a shallow `pre`,
`serve`-only host that passes TSX/JSX through that seam and returns the generated
source map to Vite.

The semantic contract uses the existing `data-tspack-source`,
`data-tspack-component`, and `data-tspack-symbol` fields. Locations are one-based
and workspace-relative with `/` separators. Intrinsic elements receive the
location because they survive into the DOM. Component and symbol identity are
optional and emitted only from clear local syntax. Browser hints remain
untrusted; the existing workspace validator is authoritative.

Production absence is structural, not a convention. The adapter's Vite
`apply: "serve"` gate excludes it from production builds, and the integration
fixture scans actual emitted files for all metadata fields and helper identity.
No runtime helper or production import is used.

The canonical UI context bundle implementation remains
`manifest-frontend/src/inspect/context-bundle.ts`. VS Code's CommonJS packaging
boundary consumes a mechanically generated copy. The generation command and
compile/test drift check are `node tools/generate-vscode-ui-context.mjs`; no
extension-local semantic edits are authoritative.
