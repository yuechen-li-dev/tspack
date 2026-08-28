import { Package, Targets, Workspace, define } from "tspack/manifest";

export default define(
  <Workspace name="tsc-m71" runtime="nodejs">
    <Package
      name="tsc-m71"
      version="1.0.0"
      kind="app"
      compilerPath="../../node_modules/.bin/tsc.cmd"
    >
      <Targets
        rows={[{
          name: "app",
          language: "typescript",
          compiler: "tsc",
          compilerConfig: "tsconfig.json",
          export: ".",
          entry: "src/main.ts",
          runtime: "dist/main.js",
          types: "dist/main.d.ts",
          deps: [],
          peers: [],
        }]}
      />
    </Package>
  </Workspace>,
);
