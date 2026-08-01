# Copeland TS / `tscl` M1

TSPack distinguishes a compiler from a JavaScript runtime. `compiler="tscl"`
selects Copeland TS compilation; `runtime="nodejs"` and a RunTarget with
`runtime="node"` still select Node process execution. `tscl` is never a
RunTarget runtime.

An M1 package supplies the deterministic compiler executable path:

```tsx
<Package name="app" version="1.0.0" kind="app"
  compiler="tscl" compilerPath="tools/tscl.exe">
  <Targets rows={[{ name: "app", export: ".", entry: "src/Main.ts", runtime: "dist/main.js", types: "" }]} />
  <RunTargets rows={[{ name: "start", runtime: "node", command: ["node", "dist/main.js"] }]} />
</Package>
```

`tspack build` passes a project-shaped JSON contract to `tscl build`, including
all local source modules, the Node production profile, entry export, build
fingerprint, and TSPack's already-resolved npm materialization rows. TSPack
remains authoritative for `ts-lock.toml`, resolution, and `sync`; Copeland does
not invoke npm or infer dependencies from `node_modules`.

The compiler result is a JSON manifest with diagnostics and output hashes. The
build identity includes `tscl`, its reported version, source bytes, entry/output
selection, production Node profile, and resolved npm package versions. If a
`tscl` build fails, TSPack removes the declared Node entry artifact so a later
`tspack run start` cannot launch a stale successful entry.

The cross-repository fixture is [`fixtures/tscl-m1`](../fixtures/tscl-m1). It
builds two Copeland TS modules and runs `Hello, TSPack` through the existing
Node RunTarget.

Existing packages omit `compiler` and retain their current `tsc` behavior.
`tspack build` intentionally does not reroute them: their existing declared
RunTarget commands remain unchanged.

Deferred: Vite/browser integration, Bun/Deno proof, CLR build/test
orchestration, sidecars, npm/native publication, watch/HMR, source maps,
remote caching, custom npm resolution, and a generalized compiler plugin model.

## Copeland attachment-plan materialization

Browser `tscl` outputs include `attachments.json`, the versioned transport form
of Copeland attachment MIR. TSPack validates schema v1, required identity and
lifecycle fields, and duplicate IDs; it records the artifact SHA-256 in
`browser-materialization.json` and emits `attachment-plan-loader.js`. The
loader registers plans automatically after the browser entry starts. TSPack
never selects an adapter, infers a host, or reconstructs payload facts.
