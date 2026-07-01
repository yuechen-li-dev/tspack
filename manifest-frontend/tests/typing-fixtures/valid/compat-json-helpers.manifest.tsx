import { CompatFiles, JsonFile, Package, Publish, Targets, TsConfig, VSCode, Workspace, defineWorkspace } from "tspack/manifest";

export default defineWorkspace(
  <Workspace name="compat-json-helpers">
    <CompatFiles>
      <JsonFile path="tsconfig.tspack.json" value={TsConfig.manifestEditor()} />
      <JsonFile
        path="tsconfig.custom.json"
        value={TsConfig.manifestEditor({
          include: ["manifest.tsx", ".tspack/types/**/*.d.ts"],
          exclude: ["dist/**", "fixtures/**"],
        })}
      />
      <JsonFile path=".vscode/settings.json" value={VSCode.settings()} />
      <JsonFile
        path=".vscode/settings.custom.json"
        value={VSCode.settings({
          typescriptTsdk: "node_modules/typescript/lib",
          "editor.defaultFormatter": "biomejs.biome",
        })}
      />
      <JsonFile path=".vscode/extensions.json" value={VSCode.extensions()} />
      <JsonFile
        path=".vscode/extensions.custom.json"
        value={VSCode.extensions({
          recommendations: ["biomejs.biome"],
          unwantedRecommendations: ["some.unwanted-extension"],
        })}
      />
    </CompatFiles>
    <Package name="compat-json-helpers" version="0.1.0" kind="app">
      <Targets rows={[]} />
      <Publish include={[]} />
    </Package>
  </Workspace>
);
