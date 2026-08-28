import { Env, Package, RunTargets, Workspace, define } from "tspack/manifest";

export default define(
  <Workspace name="inspect-live-m72">
    <Package name="inspect-live-m72" version="1.0.0" kind="app">
      <RunTargets
        rows={[
          {
            name: "dev",
            runtime: "system",
            command: ["node", "server.js"],
            cwd: "package",
            url: "http://127.0.0.1:${PORT}",
            ready: { kind: "http", path: "/" },
            env: [
              Env("PORT", {
                default: "5198",
                description: "HTTP port for the bounded inspect fixture",
              }),
            ],
          },
        ]}
      />
    </Package>
  </Workspace>,
);
