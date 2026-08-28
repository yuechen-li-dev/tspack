import { define } from "tspack/manifest";

export default define(
  <Workspace name="copeland-targets-m71c" runtime="nodejs">
    <Package
      name="copeland-targets-m71c"
      version="1.0.0"
      kind="app"
      compiler="tscl"
      compilerPath="../../../Copeland/src/Copeland/Copeland.Cli/bin/Debug/net10.0/Copeland.Cli.exe"
    >
      <Targets rows={[
        { name: "app-js", artifact: "javaScript", export: ".", entry: "src/Main.ts", runtime: "dist/js/app.js", types: "" },
        { name: "app-clr", artifact: "managedExecutable", targetFramework: "net10.0", export: "./clr", entry: "src/Main.ts", runtime: "dist/clr/app.dll", types: "" },
        { name: "app-native", artifact: "nativeExecutable", targetFramework: "net10.0", runtimeIdentifier: "win-x64", export: "./native", entry: "src/Main.ts", runtime: "dist/native/app.exe", types: "" },
        { name: "app-wasm", artifact: "wasmModule", targetFramework: "net10.0", export: "./wasm", entry: "src/Main.ts", runtime: "dist/wasm/app.wasm", types: "" },
      ]} />
      <RunTargets rows={[
        { name: "run-js", runtime: "node", cwd: "package", command: ["node", "dist/js/app.js"] },
        { name: "run-clr", runtime: "system", cwd: "package", command: ["dotnet", "dist/clr/app.dll"] },
        { name: "run-native", runtime: "system", cwd: "package", command: ["dist/native/app.exe"] },
      ]} />
      <Publish include={[]} />
    </Package>
  </Workspace>,
);
