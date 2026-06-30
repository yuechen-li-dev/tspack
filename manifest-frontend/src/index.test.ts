import fs from 'node:fs';
import os from 'node:os';
import path from 'node:path';
import { describe, expect, it } from 'vitest';
import { parsePackageManifestFile, parseWorkspace } from './index.js';

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
