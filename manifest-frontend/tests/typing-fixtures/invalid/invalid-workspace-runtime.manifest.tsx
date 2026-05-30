import { Package, Targets, Workspace, define } from 'tspack/manifest';

export default define(
  <Workspace name="mono" runtime="npm">
    <Package name="@acme/app" version="1.0.0" kind="app">
      <Targets rows={[{ name: 'web', entry: 'src/main.ts', runtime: 'dist/main.js' }]} />
    </Package>
  </Workspace>,
);
