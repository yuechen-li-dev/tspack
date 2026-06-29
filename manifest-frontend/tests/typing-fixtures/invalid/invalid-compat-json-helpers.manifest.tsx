import { CompatFiles, JsonFile, Package, Publish, Targets, VSCode, Workspace, defineWorkspace } from "tspack/manifest";

export default defineWorkspace(
  <Workspace name="invalid-compat-json-helpers">
    <CompatFiles>
      <JsonFile path="bad-settings.json" value={VSCode.settings({ broken: () => "not-json" })} />
      <JsonFile path="bad-extensions.json" value={VSCode.extensions({ recommendations: [123] })} />
    </CompatFiles>
    <Package name="invalid-compat-json-helpers" version="0.1.0" kind="app">
      <Targets rows={[]} />
      <Publish include={[]} />
    </Package>
  </Workspace>
);
