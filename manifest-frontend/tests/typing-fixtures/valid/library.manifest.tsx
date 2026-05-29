import {
  Boundaries,
  Package,
  Policies,
  Publish,
  Security,
  Targets,
  Tools,
  TypePolicy,
  BoundaryPolicy,
  Workspace,
  define,
  defineDeps,
  dep,
  npm,
  peer,
  tool,
} from 'tspack/manifest';

const typesPolicy: TypePolicy = {
  mode: 'strict',
  strict: true,
};

const boundaryPolicy: BoundaryPolicy = {
  mode: 'enforce',
};

const deps = defineDeps({
  react: peer(npm('react', '^19.0.0')),
  ts: tool(npm('typescript', '^5.9.0')),
  tslib: dep(npm('tslib', '^2.8.0')),
});

export default define(
  <Workspace name="mono">
    <Security
      acknowledgedCapabilities={[
        {
          package: 'npm:esbuild@0.24.0',
          kind: 'lifecycleScript',
          script: 'postinstall',
          command: 'node install.js',
          reason: 'Known lifecycle capability; execution remains blocked by TSPack.',
        },
      ]}
    />
    <Package name="@acme/pkg" version="1.0.0" kind="library" license="MIT" dependencies={{ values: [deps.react, deps.ts, deps.tslib] }}>
      <Policies types={typesPolicy} boundaries={boundaryPolicy} />
      <Targets
        rows={[
          {
            name: 'core',
            export: '.',
            entry: 'src/index.ts',
            runtime: 'dist/index.js',
            types: 'dist/index.d.ts',
            deps: [deps.tslib],
            peers: [deps.react],
            optional: false,
          },
        ]}
      />
      <Tools values={[deps.ts]} />
      <Boundaries rows={[{ from: 'src', to: 'test', allow: ['@acme/*'] }, { transitiveFrom: 'src/index.ts', deny: ['react-dom'], allowOnly: ['react'], denyTypeDeps: ['react-dom'] }]} />
      <Publish include={['dist/**']} exclude={['**/*.map']} />
    </Package>
  </Workspace>,
);
