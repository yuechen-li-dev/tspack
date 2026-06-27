import { Env, Package, RunTargets, Service, Workspace, define } from 'tspack/manifest';

export default define(
  <Workspace name="acme">
    <Package name="@acme/api" version="1.0.0" kind="service" license="MIT">
      <RunTargets
        rows={[
          {
            name: 'dev',
            runtime: 'node',
            command: ['tsx', 'src/server.ts'],
            url: 'http://127.0.0.1:3000',
            env: [Env('DATABASE_URL', { required: true, secret: true })],
            requires: [Service('postgres', { tcp: '127.0.0.1:5432' })],
          },
        ]}
      />
    </Package>
  </Workspace>,
);
