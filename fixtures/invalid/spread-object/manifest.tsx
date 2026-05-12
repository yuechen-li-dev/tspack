import { define } from "tspack/manifest";

const common = { name: "core" };

export default define(
  <Workspace name="mono">
    <Package name="pkg" version="1.0.0" kind="library" dependencies={{ values: [] }}>
      <Targets rows={[{ ...common, export: ".", entry: "src/index.ts", runtime: "dist/index.js", types: "dist/index.d.ts", peers: [], deps: [] }]} />
      <Publish include={["dist/**"]} exclude={[]} />
    </Package>
  </Workspace>,
);
