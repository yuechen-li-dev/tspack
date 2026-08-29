import fs from 'node:fs';
import os from 'node:os';
import path from 'node:path';
import { describe, expect, it } from 'vitest';
import { analyzeDependencySource, parsePackageManifestFile, parseWorkspace } from './index.js';

function writeFixture(name: string, contents: string): string {
  const dir = fs.mkdtempSync(path.join(os.tmpdir(), 'tspack-manifest-'));
  const file = path.join(dir, name);
  fs.writeFileSync(file, contents, 'utf8');
  return file;
}

describe('package annotation manifests', () => {
  it('parses annotatePackage as a package annotation and not a full package', () => {
    const file = writeFixture('package.manifest.tsx', `
      import { PackageAnnotations, annotatePackage, defineDeps, npm, peer, tool } from "tspack/manifest";
      const deps = defineDeps({
        react: peer(npm("react", "^19.0.0")),
        typescript: tool(npm("typescript", "^5.9.0")),
      });
      export default annotatePackage(
        <PackageAnnotations dependencies={{ values: [deps.react, deps.typescript] }} />,
      );
    `);

    const result = parsePackageManifestFile(file);

    expect(result.ok).toBe(true);
    expect(result.ir?.packages).toEqual([]);
    expect(result.ir?.packageAnnotations?.[0]?.dependencies).toHaveLength(2);
  });

  it('rejects annotation package manifests in split workspace full package rows', () => {
    const dir = fs.mkdtempSync(path.join(os.tmpdir(), 'tspack-split-'));
    fs.mkdirSync(path.join(dir, 'packages/ui'), { recursive: true });
    fs.writeFileSync(path.join(dir, 'manifest.tsx'), `
      import { Workspace, Packages, define } from "tspack/manifest";
      export default define(<Workspace name="demo"><Packages rows={[{ name: "ui", root: "packages/ui", manifest: "packages/ui/package.manifest.tsx" }]} /></Workspace>);
    `);
    fs.writeFileSync(path.join(dir, 'packages/ui/package.manifest.tsx'), `
      import { PackageAnnotations, annotatePackage } from "tspack/manifest";
      export default annotatePackage(<PackageAnnotations />);
    `);

    const result = parseWorkspace(path.join(dir, 'manifest.tsx'));

    expect(result.ok).toBe(false);
    expect(result.diagnostics.map((diagnostic) => diagnostic.code)).toContain('TSPACK_MANIFEST_PACKAGE_ANNOTATION_NOT_FULL_PACKAGE');
  });
});

describe('owned dependency source analysis', () => {
  it('locates one package-local literal dependency values island with UTF-8 byte ranges', () => {
    const source = `import { Package, Workspace, define, dep, npm } from "tspack/manifest";
const label = "é";
export default define(
  <Workspace name="demo">
    <Package name="app" dependencies={{ values: [
      // island comment
      dep(npm("react", "^19")),
    ] }} />
  </Workspace>,
);
`;
    const file = writeFixture('manifest.tsx', source);

    const result = analyzeDependencySource(file, 'app');

    expect(result.status).toBe('OwnedCanonical');
    expect(result.diagnostics).toEqual([]);
    expect(result.island?.elements).toHaveLength(1);
    const element = result.island!.elements[0];
    expect(Buffer.from(source, 'utf8').subarray(element.start, element.end).toString()).toBe('dep(npm("react", "^19"))');
    expect(result.manifestImport?.names).toContain('npm');
  });

  it('classifies absent, dynamic, and ambiguous package surfaces without guessing', () => {
    const absent = writeFixture('manifest.tsx', `
      import { Package, Workspace, define } from "tspack/manifest";
      export default define(<Workspace name="demo"><Package name="app" /></Workspace>);
    `);
    const absentResult = analyzeDependencySource(absent, 'app');
    expect(absentResult.status).toBe('Absent');
    const absentSource = fs.readFileSync(absent, 'utf8');
    expect(Buffer.from(absentSource, 'utf8').subarray(absentResult.insertion!.offset, absentResult.insertion!.offset + 2).toString()).toBe('/>');

    const dynamic = writeFixture('manifest.tsx', `
      import { Package, Workspace, define } from "tspack/manifest";
      const dynamicDependencies = { values: [] };
      export default define(<Workspace name="demo"><Package name="app" dependencies={dynamicDependencies} /></Workspace>);
    `);
    const dynamicResult = analyzeDependencySource(dynamic, 'app');
    expect(dynamicResult.status).toBe('UserDynamic');
    expect(dynamicResult.diagnostics[0].code).toBe('TSPACK_MANIFEST_DEPENDENCIES_DYNAMIC');

    const ambiguous = writeFixture('manifest.tsx', `
      import { Package, Workspace, define } from "tspack/manifest";
      export default define(<Workspace name="demo"><Package name="one" /><Package name="two" /></Workspace>);
    `);
    const ambiguousResult = analyzeDependencySource(ambiguous);
    expect(ambiguousResult.status).toBe('Ambiguous');
    expect(ambiguousResult.diagnostics[0].code).toBe('TSPACK_MANIFEST_DEPENDENCY_ISLAND_AMBIGUOUS');
  });

  it('distinguishes incremental annotation authority from native package ownership', () => {
    const annotation = writeFixture('package.manifest.tsx', `
      import { PackageAnnotations, annotatePackage } from "tspack/manifest";
      export default annotatePackage(
        <PackageAnnotations name="app" dependencies={{ values: [] }} />,
      );
    `);

    const result = analyzeDependencySource(annotation, 'app');

    expect(result.status).toBe('OwnedCanonical');
    expect(result.authority).toBe('annotation');
  });
});

describe('JSR dependency source', () => {
  it('normalizes jsr helper calls without a Deno project model', () => {
    const manifestPath = writeFixture('manifest.tsx', `
      import { Package, Workspace, define, dep, jsr } from "tspack/manifest";
      export default define(
        <Workspace name="demo">
          <Package name="app" dependencies={{ values: [dep(jsr("@std/path", "^1.1.0"))] }} />
        </Workspace>,
      );
    `);

    const result = parseWorkspace(manifestPath);

    expect(result.ok).toBe(true);
    expect(result.ir?.packages[0]?.dependencies).toEqual([
      expect.objectContaining({
        source: { kind: 'jsr', package: '@std/path', range: '^1.1.0' },
      }),
    ]);
  });
});

describe('workflow declarations', () => {
  it('builds inert programming-language-shaped flow declarations', () => {
    const manifestPath = writeFixture('manifest.tsx', `
      import { Audit, Branch, Build, Check, Parallel, PullRequest, Sequence, Sync, Test, Workflow, Workflows, Workspace, define } from "tspack/manifest";
      export default define(
        <Workspace name="demo">
          <Workflows rows={[
            Workflow("CI", {
              triggers: [PullRequest()],
              flow: Sequence(
                Sync(),
                Check(),
                Parallel(
                  Branch("test", Test()),
                  Branch("build", Build()),
                ),
                Audit(),
              ),
            }),
          ]} />
        </Workspace>,
      );
    `);

    const result = parseWorkspace(manifestPath);

    expect(result.ok).toBe(true);
    expect(result.ir?.workflows?.[0]).toEqual(expect.objectContaining({
      identity: 'CI',
      flow: {
        kind: 'sequence',
        children: [
          { kind: 'effect', effect: expect.objectContaining({ operation: 'sync' }) },
          { kind: 'effect', effect: expect.objectContaining({ operation: 'check' }) },
          {
            kind: 'parallel',
            children: [
              { kind: 'branch', identity: 'test', children: [{ kind: 'effect', effect: expect.objectContaining({ operation: 'test' }) }] },
              { kind: 'branch', identity: 'build', children: [{ kind: 'effect', effect: expect.objectContaining({ operation: 'build' }) }] },
            ],
          },
          { kind: 'effect', effect: expect.objectContaining({ operation: 'audit' }) },
        ],
      },
    }));
  });

  it('lowers typed values, exhaustive matching, cleanup, and finite fan-out as inert data', () => {
    const manifestPath = writeFixture('manifest.tsx', `
      import { Audit, Build, CurrentHost, Finally, ForEach, Linux, MatchResult, Pack, Sequence, Test, Windows, Workflow, Workflows, Workspace, define, On } from "tspack/manifest";
      const build = Build();
      export default define(
        <Workspace name="demo">
          <Workflows rows={[
            Workflow("M77", {
              triggers: [],
              flow: Sequence(
                build,
                MatchResult(build, {
                  succeeded: result => Pack(result.artifacts),
                  failed: () => Audit(),
                  cancelled: () => Audit(),
                  timedOut: () => Audit(),
                }),
                Finally(Test(), Audit()),
                ForEach("platform", [Linux(), Windows(), CurrentHost()], platform => On(platform, Test())),
              ),
            }),
          ]} />
        </Workspace>,
      );
    `);

    const result = parseWorkspace(manifestPath);

    expect(result.ok).toBe(true);
    const flow = result.ir?.workflows?.[0]?.flow as Record<string, any>;
    const match = flow.children[1];
    expect(match.kind).toBe('match');
    expect(match.arms.map((arm: Record<string, unknown>) => arm.kind)).toEqual(['succeeded', 'failed', 'cancelled', 'timedOut']);
    expect(match.arms[0].flow.effect.inputs[0]).toEqual(expect.objectContaining({ fieldPath: ['artifacts'], category: 'artifactReference' }));
    expect(flow.children[2]).toEqual(expect.objectContaining({ kind: 'finally' }));
    expect(flow.children[3]).toEqual(expect.objectContaining({
      kind: 'forEach',
      identity: 'platform',
      items: expect.arrayContaining([expect.objectContaining({
        index: 0,
        value: { kind: 'platform', string: 'linux' },
      })]),
    }));
  });

  it('diagnoses non-exhaustive matching before Flow lowering', () => {
    const manifestPath = writeFixture('manifest.tsx', `
      import { Audit, Build, MatchResult, Workflow, Workflows, Workspace, define } from "tspack/manifest";
      export default define(
        <Workspace name="demo">
          <Workflows rows={[
            Workflow("Invalid", {
              triggers: [],
              flow: MatchResult(Build(), {
                succeeded: () => Audit(),
                failed: () => Audit(),
              }),
            }),
          ]} />
        </Workspace>,
      );
    `);

    const result = parseWorkspace(manifestPath);

    expect(result.ok).toBe(false);
    expect(result.diagnostics.map(diagnostic => diagnostic.code)).toContain('TSPACK_WORKFLOW_MATCH_NON_EXHAUSTIVE');
  });

  it('lowers bounded fan-out, transfer, and typed predicates as inert data', () => {
    const manifestPath = writeFixture('manifest.tsx', `
      import { Audit, Build, CollectAll, CurrentHost, ForEach, GreaterThan, On, Pack, ParallelForEach, Sequence, Test, Transfer, When, Windows, Workflow, Workflows, Workspace, define } from "tspack/manifest";
      const build = Build();
      const portable = Transfer(build.artifacts, Windows());
      const audit = Audit();
      export default define(
        <Workspace name="demo">
          <Workflows rows={[
            Workflow("M78", {
              triggers: [],
              flow: Sequence(
                ForEach("platform", [CurrentHost(), Windows()], platform => On(platform, Test()), {
                  mode: ParallelForEach({ concurrency: 2 }),
                  failure: CollectAll(),
                }),
                build,
                portable,
                On(Windows(), Pack(portable.artifacts)),
                audit,
                When(GreaterThan(audit.failing, 0), Audit()),
              ),
            }),
          ]} />
        </Workspace>,
      );
    `);

    const result = parseWorkspace(manifestPath);
    expect(result.ok).toBe(true);
    const flow = result.ir?.workflows?.[0]?.flow as Record<string, any>;
    expect(flow.children[0]).toEqual(expect.objectContaining({
      kind: 'forEach',
      mode: 'parallel',
      concurrency: 2,
      failurePolicy: 'collectAll',
      aggregate: expect.objectContaining({ resultType: 'test', elements: expect.any(Array) }),
    }));
    expect(flow.children[2].effect).toEqual(expect.objectContaining({
      operation: 'transfer',
      transferTarget: 'windows',
      inputs: [expect.objectContaining({ category: 'artifactReference' })],
    }));
    expect(flow.children[5]).toEqual(expect.objectContaining({
      kind: 'when',
      predicate: expect.objectContaining({ kind: 'greaterThan', number: 0 }),
    }));
  });

  it('normalizes semantic workflow intent without evaluating effects', () => {
    const manifestPath = writeFixture('manifest.tsx', `
      import { Audit, Build, Check, CurrentHost, Job, Package, Process, PullRequest, Push, Secret, Sync, Test, Workflow, WorkflowEnv, Workflows, Workspace, define } from "tspack/manifest";
      export default define(
        <Workspace name="demo">
          <Workflows rows={[
            Workflow("CI", {
              triggers: [Push({ branches: ["main"] }), PullRequest({ paths: ["src/**"] })],
              jobs: [Job("test", {
                runsOn: CurrentHost(),
                steps: [
                  Sync(),
                  Check(),
                  Test({ filter: "unit" }),
                  Build({ packages: ["app"], targets: ["browser"] }),
                  Audit({ auditLevel: "high", requireCoverage: true }),
                  Process("verify", {
                    command: ["node", "--version"],
                    cwd: "workspace",
                    env: [WorkflowEnv("TOKEN", Secret("CI_TOKEN"))],
                    capabilities: ["process", "workspaceRead", "environment", "secrets"],
                  }),
                ],
              })],
            }),
          ]} />
          <Package name="app" version="1.0.0" kind="app" />
        </Workspace>,
      );
    `);

    const result = parseWorkspace(manifestPath);

    expect(result.ok).toBe(true);
    expect(result.ir?.workflows).toEqual([
      expect.objectContaining({
        identity: 'CI',
        triggers: [
          { branches: ['main'], kind: 'push' },
          { kind: 'pullRequest', paths: ['src/**'] },
        ],
        jobs: [
          expect.objectContaining({
            identity: 'test',
            runsOn: 'currentHost',
            steps: expect.arrayContaining([
              expect.objectContaining({ operation: 'sync' }),
              expect.objectContaining({ operation: 'check' }),
              expect.objectContaining({ operation: 'test', filter: 'unit' }),
              expect.objectContaining({ operation: 'build', packages: ['app'], targets: ['browser'] }),
              expect.objectContaining({ operation: 'audit', auditLevel: 'high', requireCoverage: true }),
              expect.objectContaining({ operation: 'process', command: ['node', '--version'] }),
            ]),
          }),
        ],
      }),
    ]);
  });
});
