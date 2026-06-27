import { Package, RunTargets, Service, Workspace, define } from 'tspack/manifest';

export default define(
  <Workspace name="demo">
    <Package name="app" version="1.0.0" kind="app" license="MIT">
      <RunTargets
        rows={[
          {
            name: 'dev',
            runtime: 'system',
            command: ['node', 'server.js'],
            url: 'http://127.0.0.1:3000',
            requires: [
              Service('postgres', { tcp: '127.0.0.1:5432', description: 'Local Postgres database' }),
              Service('api', { http: 'http://127.0.0.1:8080/health', expectStatus: 200, optional: true, timeoutMs: 1000 }),
            ],
          },
        ]}
      />
    </Package>
  </Workspace>,
);
