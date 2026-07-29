import { define } from "tspack/manifest";

export default define(
  <Workspace name="tscl-m1" runtime="nodejs">
    <Package
      name="tscl-m1"
      version="1.0.0"
      kind="app"
      compiler="tscl"
      compilerPath="../../../Copeland/src/Copeland/Copeland.Cli/bin/Debug/net10.0/Copeland.Cli.exe"
    >
      <Targets rows={[{ name: "app", export: ".", entry: "src/Main.ts", runtime: "dist/main.js", types: "" }]} />
      <RunTargets rows={[{ name: "start", runtime: "node", cwd: "package", command: ["node", "dist/main.js"] }]} />
      <Publish include={[]} />
    </Package>
  </Workspace>,
);
