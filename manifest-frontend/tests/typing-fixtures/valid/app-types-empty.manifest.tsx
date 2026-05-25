import { Package, RunTargets, Targets, Workspace, define } from 'tspack/manifest';

export default define(
  <Workspace name="mono">
    <Package name="@acme/app" version="1.0.0" kind="app">
      <Targets
        rows={[
          {
            name: 'web',
            entry: 'src/main.ts',
            runtime: 'dist/main.js',
            types: '',
          },
        ]}
      />
      <RunTargets
        rows={[
          {
            name: 'dev',
            runtime: 'node',
            command: ['node', 'dist/main.js'],
            url: 'http://localhost:3000',
            ready: { kind: 'http', path: '/health' },
          },
        ]}
      />
    </Package>
  </Workspace>,
);
