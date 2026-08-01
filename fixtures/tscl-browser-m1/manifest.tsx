import { define, dep, npm, path, Policies, tool, Tools } from "tspack/manifest";

const deps = {
  nanoid: dep(npm("nanoid", "^5.1.6")),
  react: dep(npm("react", "19.2.7")),
  reactDom: dep(npm("react-dom", "19.2.7")),
  vite: tool(npm("vite", "5.4.19")),
  browserHost: dep(path("runtime"), { key: "@copeland/browser-v1" }),
};

export default define(
  <Workspace name="tscl-browser-m1" runtime="nodejs">
    <Package
      name="tscl-browser-m1"
      version="1.0.0"
      kind="app"
      compiler="tscl"
      compilerPath="../../../Copeland/src/Copeland/Copeland.Cli/bin/Debug/net10.0/Copeland.Cli.exe"
      dependencies={{ values: [deps.nanoid, deps.react, deps.reactDom, deps.browserHost, deps.vite] }}
      devBackend={{
        kind: "aspnet",
        command: ["dotnet", "run", "--project", "Host/Host.csproj", "--no-launch-profile"],
        url: "http://127.0.0.1:5187",
        cwd: "package",
        ready: { kind: "http", path: "/api/status" },
        env: [
          { name: "ASPNETCORE_URLS", default: "http://127.0.0.1:5187" },
        ],
        ownsProcess: true,
        proxyRoutes: [
          { path: "/api" },
          { path: "/hub", webSocket: true },
        ],
      }}
    >
      <Policies types={{ missingTypes: "ignore" }} />
      <Tools values={[deps.vite]} />
      <Targets rows={[{
        name: "browser",
        export: ".",
        entry: "src/Main.ts",
        runtime: "dist/browser/main.js",
        types: "dist/browser/main.d.ts",
        javascriptRuntime: "browser",
        deps: [deps.nanoid, deps.react, deps.reactDom, deps.browserHost],
        npmContracts: [
          {
            package: "nanoid",
            exports: [{ name: "nanoid", parameters: [], result: "string" }],
          },
          {
            package: "react",
            exports: [],
          },
          {
            package: "react-dom/client",
            exports: [],
          },
        ],
      }]} />
      <Publish include={[]} />
    </Package>
  </Workspace>,
);
