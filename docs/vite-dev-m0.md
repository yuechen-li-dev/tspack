# TSPACK-VITE-DEV-M0

## Supported command

For a TSPack package with `compiler="tscl"` and one browser target, use:

```text
tspack run dev
```

TSPack resolves the normal manifest, starts from the existing locked and
materialized package graph, invokes the ordinary Copeland browser build, then
writes `.tspack/vite-dev/vite.config.mjs` and `supervisor.mjs`. These are
generated infrastructure files; user-authored Vite configuration is not
modified. The frontend is served at `http://127.0.0.1:5173`. The configured
port is strict, so a conflict is reported rather than silently moving a
browser scenario to another URL.

Vite serves `.tspack/vite-dev/served`, a private successful-copy of the normal
materialization directory. That keeps Windows Vite file handles away from the
compiler's canonical output while preserving the exact emitted runtime and
artifact bytes. The copy updates only after a successful build.

Vite is a TSPack tool dependency. The browser fixture pins `vite@5.4.19`; a
project must declare its own TSPack-managed Vite tool and run `tspack update`
and `tspack sync` before development. `run dev` never edits package manifests
or resolves arbitrary global Vite installations.

## Boundary and generated topology

```text
Copeland source -> tspack build -> Copeland browser artifacts -> Vite
                                     ^                        |
                                     |----- full reload -------|
```

The build is the same normal browser realization: generated JavaScript,
canonical `@copeland/browser-v1`, `attachments.json`, component-frame V1, and
materialized package graph. The generated Vite config aliases only the already
materialized canonical runtime and serves the output directory. It does not
parse Copeland source, select package contracts, or infer attachment/component
semantics.

**Vite serves and bundles browser output. It does not define Copeland
semantics.** TSPack owns the project and process lifecycle. Development and
production consume the same Copeland-emitted meaning.

## Watch and reload behavior

The TSPack-owned supervisor watches `src/` and `manifest.tsx`, debounces for
150ms, invokes `tspack build --preserve-last-successful`, and touches a private
reload marker only after a successful materialization. Its Vite infrastructure
plugin converts that marker into a full browser reload. A failed compile prints
the Copeland diagnostics and leaves the last successful browser output served;
a corrected save rebuilds and reloads without restarting Vite.

Full reload is the canonical M0 behavior. Vite HMR is a delivery mechanism;
Copeland semantic hot reload (including component-state preservation) remains
a separate future feature. Generated browser output is excluded from the
Copeland watch set, preventing feedback loops.

## Optional development backend and proxy

Browser-only packages omit `devBackend` entirely. A package that needs a local
backend declares an explicit process and proxy contract in `manifest.tsx`:

```tsx
devBackend={{
  kind: "aspnet",
  command: ["dotnet", "run", "--project", "Host/Host.csproj", "--no-launch-profile"],
  url: "http://127.0.0.1:5187",
  cwd: "package",
  ready: { kind: "http", path: "/api/status" },
  env: [{ name: "ASPNETCORE_URLS", default: "http://127.0.0.1:5187" }],
  ownsProcess: true,
  proxyRoutes: [
    { path: "/api" },
    { path: "/hub", webSocket: true },
  ],
}}
```

`kind` is currently `process` or the first dogfood profile, `aspnet`.
`ownsProcess: true` makes TSPack start the declared argv, wait for its distinct
readiness check, then start Vite. Vite receives generated proxy entries only;
routes without a `target` use the resolved backend URL. Route targets may use
the same non-secret environment placeholders as readiness URLs. `webSocket: true`
maps to Vite's `ws` proxy flag. `secure` is emitted explicitly; the
fixture proves HTTP with `secure: false`, not HTTPS certificates or WSS.

`--env VITE_PORT=5190` selects an isolated strict frontend port for a test or
parallel developer session. The configured backend URL remains a manifest
fact. All machine-specific URLs are confined to `.tspack/vite-dev` and never
enter production output.

**TSPack owns backend and frontend development orchestration.** **Vite proxies
requests; it does not define the backend contract.** Backend startup/readiness
errors are reported before Vite starts, and TSPack only stops child processes
it started. A failed Copeland build leaves the last successful application
running. Full reload is the M0 development law; semantic hot reload remains
separate.

## Responsibility audit

| Responsibility | M0 owner | Status |
| --- | --- | --- |
| Copeland source graph, JS, attachment/frame artifacts | Copeland compiler | unchanged |
| manifest, npm/materialization, build invocation, watching, child cleanup | TSPack | canonical |
| ESM delivery, source maps where emitted, CSS/assets, refresh | Vite | canonical dev host |
| raw static server used by prior browser fixture | compatibility/test-only | retained temporarily |
| ASP.NET process launch and API/WebSocket proxy | TSPack manifest + generated Vite config | M0 HTTP proof; WebSocket configuration support |

The old `python -m http.server` fixture instruction was an ad-hoc static-host
proof, not a second semantic runtime. It remains suitable for production-like
static smoke coverage only; it is not the promoted development command.

## Production and compatibility

Production remains the existing deterministic Copeland browser materialization
path; it does not run the Vite dev server or embed proxy URLs. The raw static
HTTP path is retained only for production-like compatibility smoke coverage.
Source maps are served transparently when Copeland emits them, but M0 does not
claim source-map composition beyond the compiler's existing output.
