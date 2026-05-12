import { defineWorkspace } from "tspack/manifest";
export default defineWorkspace(
  <Workspace name="m6b-workspace">
    <Packages rows={[
      { name: "@m6b/core", root: "packages/core", manifest: "packages/core/package.manifest.tsx" },
      { name: "@m6b/react", root: "packages/react", manifest: "packages/react/package.manifest.tsx" }
    ]} />
  </Workspace>
);
