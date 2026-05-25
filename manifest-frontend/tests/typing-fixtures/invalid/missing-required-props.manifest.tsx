import { Package, Targets, Workspace, define } from 'tspack/manifest';

export default define(
  <Workspace name="mono">
    <Package kind="app">
      <Targets rows={[{ name: 'core', types: 'dist/index.d.ts' }]} />
    </Package>
  </Workspace>,
);
