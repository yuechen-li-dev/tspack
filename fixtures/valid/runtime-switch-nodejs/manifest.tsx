import { define, defineDeps, dep, npm } from "tspack/manifest";

const deps = defineDeps({
  leftPad: dep(npm("left-pad", "^1.3.0")),
});

export default define(
  <Workspace name="runtime-switch" runtime="nodejs">
    <Package
      name="runtime-switch"
      version="0.1.0"
      license="MIT"
      kind="library"
      dependencies={{ values: [deps.leftPad] }}
    >
      <Targets
        rows={[
          {
            name: "core",
            export: ".",
            entry: "src/index.ts",
            runtime: "dist/index.js",
            types: "dist/index.d.ts",
            deps: ["left-pad"],
            peers: [],
          },
        ]}
      />
      <RunTargets
        rows={[
          {
            name: "node-hello",
            runtime: "node",
            cwd: "workspace",
            command: ["node", "scripts/node-hello.js", "from-node"],
            ready: { kind: "stdout-match", pattern: "ready", stream: "stdout" },
          },
          {
            name: "bun-hello",
            runtime: "bun",
            cwd: "workspace",
            command: ["scripts/bun-hello.js", "from-bun"],
            ready: { kind: "stdout-match", pattern: "ready", stream: "stdout" },
          },
          {
            name: "deno-hello",
            runtime: "deno",
            cwd: "workspace",
            command: ["run", "scripts/deno-hello.ts", "from-deno"],
            ready: { kind: "stdout-match", pattern: "ready", stream: "stdout" },
          },
          {
            name: "system-hello",
            runtime: "system",
            cwd: "workspace",
            command: ["scripts/system-hello", "from-system"],
            ready: { kind: "stdout-match", pattern: "ready", stream: "stdout" },
          },
        ]}
      />
      <Publish include={["dist/**", "README.md", "LICENSE"]} exclude={[]} />
    </Package>
  </Workspace>,
);
