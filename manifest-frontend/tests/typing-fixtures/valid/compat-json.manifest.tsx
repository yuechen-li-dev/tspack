import { CompatFiles, JsonFile, Package, Publish, Targets, Workspace, defineWorkspace, json } from "tspack/manifest";

export default defineWorkspace(
  <Workspace name="compat-json">
    <CompatFiles>
      <JsonFile
        path="tsconfig.tspack.json"
        value={{
          include: ["manifest.tsx"],
          compilerOptions: {
            jsx: "react-jsx",
          },
        }}
      />
      <JsonFile path="array.json" value={json(["one", true, null])} />
      <JsonFile path="scalar.json" value="ok" />
    </CompatFiles>
    <Package name="compat-json" version="0.1.0" kind="app">
      <Targets rows={[]} />
      <Publish include={[]} />
    </Package>
  </Workspace>
);
