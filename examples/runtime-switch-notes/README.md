# Runtime Switch Notes

Runtime Switch Notes is a deliberately small TSPack dogfooding project. It exists to exercise current TSPack contracts rather than to demonstrate UI design.

The page contains:

- a `Runtime Switch Notes` heading
- a `section` labelled `Runtime status`
- Node.js, Bun, and Deno runtime rows
- a `New note` button
- two sample notes
- source hints through `data-tspack-source`, `data-tspack-component`, and `data-tspack-symbol`

## Project shape

```text
examples/runtime-switch-notes/
  manifest.tsx
  src/ui/                # packable UI library source
  src/app/               # tiny browser mount source
  dist/ui/               # checked-in library output for pack smoke tests
  public/                # static app served by RunTargets
  server/                # Node.js, Bun, and Deno RunTargets
  tests/                 # native xTest and inspect-helper smoke tests
```

The manifest uses two packages in one workspace:

- `@tspack-examples/runtime-switch-ui` is a packable library package.
- `@tspack-examples/runtime-switch-app` is an app package that depends on the UI package by `workspace(...)` and owns the run targets.

This keeps the sample small while still giving `tspack why` a workspace edge to explain.

## Runtime profile switch

The workspace intentionally starts as Node.js:

```tsx
<Workspace name="runtime-switch-notes" runtime="nodejs">
```

The one-line runtime-profile variants are:

```tsx
<Workspace name="runtime-switch-notes" runtime="nodejs">
<Workspace name="runtime-switch-notes" runtime="bun">
<Workspace name="runtime-switch-notes" runtime="deno">
```

The runtime profile is not a package-manager switch. RunTargets stay explicit about the runtime they launch.

## Run targets

List the targets:

```sh
go run ./cmd/tspack run --root examples/runtime-switch-notes --list
```

From this directory, the declared targets are:

```sh
go run ./cmd/tspack run --root examples/runtime-switch-notes node-server --once
go run ./cmd/tspack run --root examples/runtime-switch-notes bun-server --once
go run ./cmd/tspack run --root examples/runtime-switch-notes deno-server --once
```

Ports are fixed to avoid accidental conflicts:

- Node.js: <http://127.0.0.1:4171>
- Bun: <http://127.0.0.1:4172>
- Deno: <http://127.0.0.1:4173>

The Node.js target uses `runtime: "node"`, `cwd: "workspace"`, a shebang executable command, and stdout readiness. The Bun and Deno targets use `runtime: "bun"` and `runtime: "deno"`, so TSPack prefixes the runtime executable and reports missing-runtime diagnostics when those tools are unavailable.

## Inspect and xTest

The rendered app includes source hints for inspect tooling:

```html
data-tspack-source="src/app/index.ts:12:3"
data-tspack-component="RuntimeSwitchNotes"
data-tspack-symbol="RuntimeSwitchNotes.App"
```

Native xTest coverage includes:

- pure render assertions
- a stable HTML snapshot
- a static `assert.type` assertion for the exported UI model
- inspect-helper assertions against a representative source-hinted tree
- a stable JSON snapshot of selected inspect data

Run the tests:

```sh
go run ./cmd/tspack test --root examples/runtime-switch-notes
go run ./cmd/tspack test --root examples/runtime-switch-notes --compact
go run ./cmd/tspack test --root examples/runtime-switch-notes --batch
```

Live browser inspect is intentionally documented as a manual smoke because Playwright browsers may not be installed on every contributor machine. URL inspect uses the Playwright URL backend by default and should not require VS Code; a missing browser runtime should report a browser/backend diagnostic instead of `TSPACK_INSPECT_VSCODE_NOT_FOUND`.

```sh
go run ./cmd/tspack run --root examples/runtime-switch-notes node-server
# in another shell
go run ./cmd/tspack inspect http://127.0.0.1:4171 --json
go run ./cmd/tspack inspect http://127.0.0.1:4171 --selector main --json
```

## Pack, why, security, format, and lint

The UI package is packable without running a build because `dist/ui/**` is checked in as fixture output:

```sh
go run ./cmd/tspack pack --root examples/runtime-switch-notes --package @tspack-examples/runtime-switch-ui --dry-run
go run ./cmd/tspack pack --root examples/runtime-switch-notes --package @tspack-examples/runtime-switch-ui --verify
```

Explain the app-to-UI workspace dependency:

```sh
go run ./cmd/tspack why --root examples/runtime-switch-notes @tspack-examples/runtime-switch-ui
go run ./cmd/tspack why --root examples/runtime-switch-notes @tspack-examples/runtime-switch-ui --json
```

The sample has no package lifecycle scripts. Security checks should therefore report the no-capability/clean path:

```sh
go run ./cmd/tspack doctor security --root examples/runtime-switch-notes
```

No `biome.json` is included. The dogfooding intent is to exercise TSPack's default Biome-backed format/lint signaling rather than to hide defaults behind local config:

```sh
go run ./cmd/tspack format --root examples/runtime-switch-notes --check
go run ./cmd/tspack lint --root examples/runtime-switch-notes
go run ./cmd/tspack check --root examples/runtime-switch-notes --format
```

## Known limitations

- The Deno server uses a fixed port rather than reading `PORT`, so it does not need `--allow-env` just to demonstrate a static file server.
- Live inspect is manual because requiring Playwright browsers would make the example brittle in CI.
- The app is intentionally not built by TSPack; checked-in `dist/ui/**` and `public/app.js` make pack and run surfaces deterministic for this smoke.
