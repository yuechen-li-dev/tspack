import {
  CompatFiles,
  JsonFile,
  Package,
  Publish,
  Targets,
  TsConfig,
  VSCode,
  Workspace,
  defineWorkspace,
  json,
} from "tspack/manifest";

export default defineWorkspace(
  <Workspace name="compat-json-basic">
    <CompatFiles>
      <JsonFile path="tsconfig.tspack.json" value={TsConfig.manifestEditor()} />
      <JsonFile
        path=".vscode/settings.json"
        value={VSCode.settings({
          "editor.defaultFormatter": "biomejs.biome",
          "typescript.tsdk": "node_modules/typescript/lib",
        })}
      />
      <JsonFile
        path=".vscode/extensions.json"
        value={VSCode.extensions({
          recommendations: ["biomejs.biome"],
        })}
      />
      <JsonFile path="compat.raw.json" value={json({ raw: true })} />
    </CompatFiles>
    <Package name="compat-json-basic" version="0.1.0" kind="app">
      <Targets rows={[]} />
      <Publish include={[]} />
    </Package>
  </Workspace>
);
