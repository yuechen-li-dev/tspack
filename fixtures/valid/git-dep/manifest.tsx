import { define, defineDeps, dep, git } from "tspack/manifest";

const deps = defineDeps({
  helper: dep(git("github:acme/helper", { tag: "v1.2.0" })),
});

export default define(
  <Workspace name="mono">
    <Package name="gitpkg" version="0.1.0" license="MIT" kind="library" dependencies={{ values: [deps.helper] }}>
      <Targets rows={[{ name: "core", export: ".", entry: "src/index.ts", runtime: "dist/index.js", types: "dist/index.d.ts", peers: [], deps: [], optional: false }]} />
      <Publish include={["dist/**"]} exclude={[]} />
    </Package>
  </Workspace>,
);
