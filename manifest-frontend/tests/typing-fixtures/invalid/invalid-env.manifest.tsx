import { Env, Package, RunTargets, Workspace, define } from 'tspack/manifest';

export default define(
  <Workspace name="env-ws">
    <Package name="app" version="1.0.0" kind="app">
      <RunTargets
        rows={[
          {
            name: 'dev',
            command: ['node', 'server.js'],
            url: 'http://127.0.0.1:3000',
            env: [Env('PORT', { default: 3000 })],
          },
        ]}
      />
    </Package>
  </Workspace>,
);
