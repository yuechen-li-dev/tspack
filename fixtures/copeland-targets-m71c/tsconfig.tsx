import { defineTypeScriptWorkspace } from "copeland/workspace";

export default defineTypeScriptWorkspace({
  ownership: "partial",
  tscl: {
    project: "./App.csproj",
    include: ["src/**"],
    targets: {
      "app-js": { backend: "javascript", runtime: "node" },
      "app-clr": { backend: "csharp", runtime: "ryujit", targetFramework: "net10.0" },
      "app-native": { backend: "csharp", runtime: "nativeaot", targetFramework: "net10.0", runtimeIdentifier: "win-x64" },
      "app-wasm": { backend: "csharp", runtime: "wasm", targetFramework: "net10.0" },
    },
  },
});
