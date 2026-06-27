import { Package, Workspace, define } from 'tspack/manifest';

export default define(
  <Workspace name="acme">
    <Package name="@acme/worker" version="1.0.0" kind="worker" />
  </Workspace>,
);
