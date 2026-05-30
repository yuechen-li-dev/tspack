import { Package, RunTargets, Workspace, define } from 'tspack/manifest';

export default define(
  <Workspace name="mono">
    <Package name="@acme/app" version="1.0.0" kind="app">
      <RunTargets
        rows={[
          {
            name: 'dev',
            runtime: 'deno',
            command: ['server.ts'],
            url: 'http://localhost:3000',
          },
        ]}
      />
    </Package>
  </Workspace>,
);
