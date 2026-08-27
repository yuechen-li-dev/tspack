import { define, defineDeps, dep, jsr, npm } from "tspack/manifest";

const deps = defineDeps({
  colors: dep(npm("picocolors", "^1.1.1")),
  flag: dep(jsr("@luca/flag", "^1.0.1")),
  path: dep(jsr("@std/path", "^1.1.6")),
  esbuildPlugin: dep(jsr("@deno/esbuild-plugin", "^1.2.1")),
});

export default define(
  <Workspace name="jsr-mixed" runtime="nodejs">
    <Package name="jsr-mixed" version="0.1.0" license="MIT" kind="library" dependencies={{ values: [deps.colors, deps.flag, deps.path, deps.esbuildPlugin] }}>
      <Targets rows={[{
        name: "core",
        export: ".",
        entry: "src/index.ts",
        runtime: "dist/index.js",
        types: "src/index.ts",
        peers: [],
        deps: ["picocolors", "@luca/flag", "@std/path", "@deno/esbuild-plugin"],
      }]} />
      <Policies types={{ declarations: "none", missingTypes: "ignore" }} />
      <Publish include={["dist/**"]} exclude={[]} />
    </Package>
  </Workspace>,
);
