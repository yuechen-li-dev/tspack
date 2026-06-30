import {
  Package,
  Publish,
  Security,
  Targets,
  UpdatePolicy,
  Workspace,
  define,
  defineDeps,
  dep,
  npm,
  peer,
  tool,
  workspace,
} from "tspack/manifest";

const deps = defineDeps({
  typescript: tool(npm("typescript", "^5.8.0")),
  vite: tool(npm("vite", "^5.4.0")),
  esbuild: tool(npm("esbuild", "^0.21.0")),
  biome: tool(npm("@biomejs/biome", "^1.9.0"), { key: "@biomejs/biome" }),
  rollup: tool(npm("rollup", "^4.20.0")),
  react: dep(npm("react", "^18.3.0")),
  reactDom: peer(npm("react-dom", "^18.3.0"), { key: "react-dom" }),
  utils: dep(workspace("@tspack-examples/update-policy-utils")),
});

export default define(
  <Workspace name="update-policy-notes" runtime="nodejs">
    <Security
      acknowledgedCapabilities={[
        {
          package: "npm:@biomejs/biome@1.10.0",
          kind: "lifecycleScript",
          script: "postinstall",
          command: "node ./scripts/postinstall.js",
          reason: "Reviewed Biome postinstall native-binary selection for this fixture.",
          behaviorFixture: "security/biome-postinstall.xtest.ts",
        },
      ]}
      acknowledgedLifecycleCategories={[
        {
          category: "maintainer-publish",
          scripts: ["prepare", "prepublishOnly"],
          reason: "Maintainer-publish scripts are reviewed as publish-time metadata and are not run by TSPack installs.",
        },
      ]}
    />
    <UpdatePolicy
      rows={[
        {
          name: "typescript",
          kind: "tool",
          strategy: "rolling",
          level: "minor",
          reason: "Compiler updates can roll within the current major after CI.",
        },
        {
          name: "vite",
          kind: "tool",
          strategy: "rolling",
          level: "minor",
          reason: "Bundler updates stay on the current major until explicitly reviewed.",
        },
        {
          name: "esbuild",
          kind: "tool",
          strategy: "rolling",
          level: "major",
          reason: "Native build tool can roll only when lifecycle security gates pass.",
        },
        {
          name: "@biomejs/biome",
          kind: "tool",
          strategy: "rolling",
          level: "minor",
          reason: "Formatter updates are accepted after exact lifecycle capability review.",
        },
        {
          name: "rollup",
          kind: "tool",
          strategy: "rolling",
          level: "minor",
          reason: "Rollup publish-time scripts are covered by category acknowledgment.",
        },
        {
          name: "react",
          kind: "dep",
          strategy: "manual",
          reason: "Runtime framework upgrades require app/library coordination.",
        },
        {
          name: "react-dom",
          kind: "peer",
          strategy: "pinned",
          reason: "Peer/runtime DOM binding remains pinned for consumer compatibility.",
        },
      ]}
    />

    <Package name="@tspack-examples/update-policy-utils" version="0.1.0" license="MIT" kind="library" dependencies={{ values: [deps.typescript, deps.biome, deps.rollup] }}>
      <Targets rows={[{ name: "utils", export: ".", entry: "src/utils.ts", runtime: "dist/utils.js", types: "dist/utils.d.ts", deps: [], peers: [] }]} />
      <Publish include={["dist/**", "README.md", "LICENSE"]} exclude={[]} />
    </Package>

    <Package name="@tspack-examples/update-policy-lib" version="0.1.0" license="MIT" kind="library" dependencies={{ values: [deps.react, deps.reactDom, deps.utils, deps.typescript, deps.vite] }}>
      <Targets rows={[{ name: "lib", export: ".", entry: "src/lib.tsx", runtime: "dist/lib.js", types: "dist/lib.d.ts", deps: ["react", "@tspack-examples/update-policy-utils"], peers: ["react-dom"] }]} />
      <Publish include={["dist/**", "README.md", "LICENSE"]} exclude={[]} />
    </Package>

    <Package name="@tspack-examples/update-policy-app" version="0.1.0" license="MIT" kind="app" dependencies={{ values: [deps.react, deps.reactDom, deps.utils, deps.typescript, deps.vite, deps.esbuild] }}>
      <Targets rows={[{ name: "app", export: ".", entry: "src/app.tsx", runtime: "public/app.js", types: "public/app.d.ts", deps: ["react", "react-dom", "@tspack-examples/update-policy-utils"], peers: [] }]} />
    </Package>
  </Workspace>,
);
