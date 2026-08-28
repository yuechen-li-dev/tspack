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
              { operation: 'sync' },
              { operation: 'check' },
              { operation: 'test', filter: 'unit' },
              { operation: 'build', packages: ['app'], targets: ['browser'] },
              { operation: 'audit', auditLevel: 'high', requireCoverage: true },
              expect.objectContaining({ operation: 'process', command: ['node', '--version'] }),
            ]),
          }),
        ],
      }),
    ]);
  });
});
