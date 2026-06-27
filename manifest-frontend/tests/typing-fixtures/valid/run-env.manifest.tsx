import { Env, Package, RunTargets, Workspace, define } from 'tspack/manifest';

export default define(
  <Workspace name="env-ws">
    <Package name="app" version="1.0.0" kind="app">
      <RunTargets
        rows={[
          {
            name: 'dev',
            runtime: 'node',
            command: ['tsx', 'server.ts'],
            url: 'http://127.0.0.1:3000',
            env: [
              Env('DATABASE_URL', { required: true, secret: true, description: 'Postgres connection string' }),
              Env('PORT', { default: '3000', description: 'HTTP port' }),
            ],
          },
        ]}
      />
    </Package>
  </Workspace>,
);
