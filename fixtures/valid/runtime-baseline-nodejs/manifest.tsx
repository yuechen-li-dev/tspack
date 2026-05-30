import { define, defineDeps, dep, npm } from "tspack/manifest";

const deps = defineDeps({
  leftPad: dep(npm("left-pad", "^1.3.0")),
});

export default define(
  <Workspace name="runtime-baseline" runtime="nodejs">
    <Package name="runtime-baseline" version="0.1.0" license="MIT" kind="library" dependencies={{ values: [deps.leftPad] }}>
      <Targets rows={[{ name: "core", export: ".", entry: "src/index.ts", runtime: "dist/index.js", types: "dist/index.d.ts", deps: ["left-pad"], peers: [] }]} />
      <RunTargets rows={[{ name: "dev", runtime: "system", cwd: "workspace", command: ["node", "scripts/dev.js"], url: "http://127.0.0.1:5173" }]} />
      <Publish include={["dist/**", "README.md", "LICENSE"]} exclude={[]} />
    </Package>
  </Workspace>,
);
