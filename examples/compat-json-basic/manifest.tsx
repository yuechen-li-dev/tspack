import { defineWorkspace } from "tspack/manifest";

export default defineWorkspace(
  <Workspace name="compat-json-basic">
    <CompatFiles>
      <JsonFile
        path="tsconfig.tspack.json"
        value={{
          extends: "./tsconfig.json",
          compilerOptions: {
            jsx: "react-jsx",
            moduleResolution: "Bundler",
          },
          include: ["manifest.tsx"],
        }}
      />
      <JsonFile
        path=".vscode/settings.json"
        value={{
          "editor.defaultFormatter": "biomejs.biome",
          "typescript.tsdk": "node_modules/typescript/lib",
        }}
      />
    </CompatFiles>
    <Package name="compat-json-basic" version="0.1.0" kind="app">
      <Targets rows={[]} />
      <Publish include={[]} />
    </Package>
  </Workspace>
);
