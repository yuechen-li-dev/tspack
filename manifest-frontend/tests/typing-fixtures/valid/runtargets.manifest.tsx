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
          {
            name: 'redis',
            runtime: 'system',
            command: ['redis-server', '--port', '6379'],
            url: 'http://localhost:6379',
            ready: { kind: 'tcp', host: '127.0.0.1', port: 6379 },
          },
          {
            name: 'vite',
            runtime: 'system',
            command: ['vite', '--host', '127.0.0.1'],
            url: 'http://localhost:5173',
            ready: { kind: 'stdout-match', pattern: 'Local:', stream: 'stdout' },
          },
          {
            name: 'dev-bun',
            runtime: 'bun',
            command: ['server.js'],
            url: 'http://localhost:3001',
            ready: { kind: 'stdout-match', pattern: 'ready', stream: 'stdout' },
          },
        ]}
      />
    </Package>
  </Workspace>,
);
