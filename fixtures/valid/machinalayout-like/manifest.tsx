import { define, defineDeps, npm, peer, tool } from "tspack/manifest";

const types = { declarations: "required", missingTypes: "error", publicTypeLeakage: "error" };
const deps = defineDeps({
  typescript: tool(npm("typescript", "^5.9")),
  vitest: tool(npm("vitest", "^3.2")),
  react: peer(npm("react", ">=18 <20")),
  reactDom: peer(npm("react-dom", ">=18 <20")),
  vue: peer(npm("vue", ">=3 <4"), { optional: true }),
});

export default define(
  <Workspace name="machina">
    <Package name="machinalayout" version="1.0.0" license="MIT" kind="library" dependencies={{ values: [deps.typescript, deps.vitest, deps.react, deps.reactDom, deps.vue] }}>
      <Policies types={types} boundaries={{ strict: "error" }} />
      <Targets rows={[
        { name: "core", export: ".", entry: "src/index.ts", runtime: "dist/index.js", types: "dist/index.d.ts", peers: [], deps: [], optional: false },
        { name: "react", export: "./react", entry: "src/react.ts", runtime: "dist/react.js", types: "dist/react.d.ts", peers: ["react", "react-dom"], deps: [], optional: false },
        { name: "vue", export: "./vue", entry: "src/vue.ts", runtime: "dist/vue.js", types: "dist/vue.d.ts", peers: ["vue"], deps: [], optional: true },
      ]} />
      <Tools values={[deps.typescript, deps.vitest]} />
      <Boundaries rows={[{ from: "src/index.ts", denyDeps: ["react", "react-dom", "vue"] }]} />
      <Publish include={["dist/**", "README.md", "LICENSE"]} exclude={["src/**", "test/**", "samples/**"]} />
    </Package>
  </Workspace>,
);
