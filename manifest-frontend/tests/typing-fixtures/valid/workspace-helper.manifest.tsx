import { Package, RegistryPolicy, RegistrySource, Targets, Workspace, define, defineDeps, dep, workspace } from 'tspack/manifest';

const deps = defineDeps({
  core: dep(workspace('@acme/core')),
});

export default define(
  <Workspace name="mono">
    <RegistryPolicy allowedSources={['npm', 'jsr']} requireIntegrity={true}>
      <RegistrySource kind="npm" endpoints={[{ url: 'https://registry.npmjs.org', tokenEnv: 'NPM_TOKEN' }]} />
    </RegistryPolicy>
    <Package name="@acme/app" version="1.0.0" kind="app" dependencies={{ values: [deps.core] }}>
      <Targets rows={[{ name: 'web', entry: 'src/main.ts', runtime: 'dist/main.js' }]} />
    </Package>
  </Workspace>,
);
