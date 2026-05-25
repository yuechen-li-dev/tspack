import { Package, Targets, Workspace, define, defineDeps, dep, workspace } from 'tspack/manifest';

const deps = defineDeps({
  core: dep(workspace('@acme/core')),
});

export default define(
  <Workspace name="mono">
    <Package name="@acme/app" version="1.0.0" kind="app" dependencies={{ values: [deps.core] }}>
      <Targets rows={[{ name: 'web', entry: 'src/main.ts', runtime: 'dist/main.js' }]} />
    </Package>
  </Workspace>,
);
