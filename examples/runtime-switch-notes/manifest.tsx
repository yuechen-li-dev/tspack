import { define, defineDeps, dep, workspace } from "tspack/manifest";

const deps = defineDeps({
  ui: dep(workspace("@tspack-examples/runtime-switch-ui")),
});

export default define(
  <Workspace name="runtime-switch-notes" runtime="nodejs">
    <Package
      name="@tspack-examples/runtime-switch-ui"
      version="0.1.0"
      license="MIT"
      kind="library"
      dependencies={{ values: [] }}
    >
      <Targets
        rows={[
          {
            name: "ui",
            export: ".",
            entry: "src/ui/index.ts",
            runtime: "dist/ui/index.js",
            types: "dist/ui/index.d.ts",
            deps: [],
            peers: [],
          },
        ]}
      />
      <Publish
        include={["dist/ui/**", "README.md", "LICENSE", "CHANGELOG.md"]}
        exclude={[]}
      />
    </Package>

    <Package
      name="@tspack-examples/runtime-switch-app"
      version="0.1.0"
      license="MIT"
      kind="app"
      dependencies={{ values: [deps.ui] }}
    >
      <Targets
        rows={[
          {
            name: "app",
            export: ".",
            entry: "src/app/index.ts",
            runtime: "public/app.js",
            types: "public/app.d.ts",
            deps: ["@tspack-examples/runtime-switch-ui"],
            peers: [],
          },
        ]}
      />
      <RunTargets
        rows={[
          {
            name: "node-server",
            runtime: "node",
            cwd: "workspace",
            command: ["server/node-server.js"],
            url: "http://127.0.0.1:4171",
            ready: { kind: "stdout-match", pattern: "ready", stream: "stdout" },
          },
          {
            name: "bun-server",
            runtime: "bun",
            cwd: "workspace",
            command: ["server/bun-server.ts"],
            url: "http://127.0.0.1:4172",
            ready: { kind: "stdout-match", pattern: "ready", stream: "stdout" },
          },
          {
            name: "deno-server",
            runtime: "deno",
            cwd: "workspace",
            command: [
              "run",
              "--allow-net=127.0.0.1:4173",
              "--allow-read=public",
              "server/deno-server.ts",
            ],
            url: "http://127.0.0.1:4173",
            ready: { kind: "stdout-match", pattern: "ready", stream: "stdout" },
          },
        ]}
      />
      <Publish
        include={["public/**", "server/**", "README.md", "LICENSE"]}
        exclude={[]}
      />
    </Package>
  </Workspace>,
);
