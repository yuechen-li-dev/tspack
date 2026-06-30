import { Package, Targets, defineDeps, definePackage, dep, npm } from "tspack/manifest";

const deps = defineDeps({
  react: dep(npm("react", "^19.0.0"), { key: "react" }),
});

export default definePackage(
  <Package
    name="@m6b/react"
    version="0.1.0"
    kind="library"
    dependencies={{ values: [deps.react] }}
  >
    <Targets
      rows={[
        {
          name: "react",
          export: ".",
          entry: "src/index.ts",
          runtime: "src/index.ts",
          types: "dist/index.d.ts",
          deps: ["react"],
          peers: ["react"],
        },
      ]}
    />
  </Package>,
);
