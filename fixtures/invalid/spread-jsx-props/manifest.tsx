import { define } from "tspack/manifest";

const pkgProps = { name: "pkg", version: "1.0.0", kind: "library" };

export default define(
  <Workspace name="mono">
    <Package {...pkgProps} dependencies={{ values: [] }}>
      <Targets rows={[{ name: "core", export: ".", entry: "src/index.ts", runtime: "dist/index.js", types: "dist/index.d.ts", peers: [], deps: [] }]} />
      <Publish include={["dist/**"]} exclude={[]} />
    </Package>
  </Workspace>,
);
