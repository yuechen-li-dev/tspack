import { CompatFiles, JsonFile, Package, Publish, Targets, Workspace, defineWorkspace } from "tspack/manifest";

export default defineWorkspace(
  <Workspace name="invalid-compat-json">
    <CompatFiles>
      <JsonFile value={{ ok: true }} />
      <JsonFile path="bad.json" />
      <JsonFile path="fn.json" value={() => "not-json"} />
    </CompatFiles>
    <Package name="invalid-compat-json" version="0.1.0" kind="app">
      <Targets rows={[]} />
      <Publish include={[]} />
    </Package>
  </Workspace>
);
