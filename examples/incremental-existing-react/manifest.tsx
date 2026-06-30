import {
  CompatFiles,
  JsonFile,
  TsConfig,
  VSCode,
  Workspace,
  defineWorkspace,
} from "tspack/manifest";

export default defineWorkspace(
  <Workspace name="incremental-existing-react">
    <CompatFiles>
      <JsonFile
        path="tsconfig.tspack.json"
        value={TsConfig.manifestEditor()}
      />
      <JsonFile
        path=".vscode/settings.json"
        value={VSCode.settings()}
      />
      <JsonFile
        path=".vscode/extensions.json"
        value={VSCode.extensions()}
      />
    </CompatFiles>
  </Workspace>,
);
