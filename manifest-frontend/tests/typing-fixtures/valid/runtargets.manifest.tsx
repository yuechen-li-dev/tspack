import { Package, RunTargets, Workspace, define } from 'tspack/manifest';

export default define(
  <Workspace name="mono">
    <Package name="@acme/app" version="1.0.0" kind="app">
      <RunTargets
        rows={[
          {
            name: 'dev-system',
            runtime: 'system',
            command: ['pnpm', 'dev'],
            cwd: 'package',
            url: 'http://localhost:5173',
          },
          {
            name: 'dev-node',
            runtime: 'node',
            command: ['node', 'dist/main.js'],
            url: 'http://localhost:3000',
            ready: { kind: 'http', path: '/ready' },
          },
        ]}
      />
    </Package>
  </Workspace>,
);
