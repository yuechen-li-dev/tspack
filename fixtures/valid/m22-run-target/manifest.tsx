import { define } from "tspack/manifest";

export default define(
  <Workspace name="mono">
    <Package name="app" version="1.0.0" kind="app" dependencies={{ values: [] }}>
      <Targets rows={[]} />
      <RunTargets
        rows={[
          {
            name: "dev",
            runtime: "system",
            command: ["node", "server.js"],
            cwd: "package",
            url: "http://127.0.0.1:5173",
            ready: { kind: "http", path: "/" },
          },
          {
            name: "api",
            runtime: "node",
            command: ["node", "api.js", "&&"],
            url: "http://127.0.0.1:5180",
          },
        ]}
      />
      <Publish include={["dist/**"]} exclude={[]} />
    </Package>
  </Workspace>,
);
