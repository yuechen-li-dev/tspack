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
              Service('postgres', {}),
              Service('api', { tcp: '127.0.0.1:8080', http: 'http://127.0.0.1:8080/health' }),
              Service('bad-status', { http: 'http://127.0.0.1', expectStatus: '200' }),
              Service('bad-timeout', { tcp: '127.0.0.1:6379', timeoutMs: '1000' }),
            ],
          },
        ]}
      />
    </Package>
  </Workspace>,
);
