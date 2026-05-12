import { define, defineDeps, npm, tool } from "tspack/manifest";

const deps = defineDeps({
  typescript: tool(npm("typescript", "^5.9")),
});

export default define(
  <Workspace name="mono">
    <Package name="minimal" version="0.1.0" license="MIT" kind="library" dependencies={{ values: [deps.typescript] }}>
      <Targets rows={[{ name: "core", export: ".", entry: "src/index.ts", runtime: "dist/index.js", types: "dist/index.d.ts", peers: [], deps: [], optional: false }]} />
      <Tools values={[deps.typescript]} />
      <Publish include={["dist/**", "README.md", "LICENSE"]} exclude={[]} />
    </Package>
  </Workspace>,
);
