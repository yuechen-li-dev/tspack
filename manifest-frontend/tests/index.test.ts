import fs from 'node:fs';
import path from 'node:path';
import { describe, expect, it } from 'vitest';
import { parseManifestFile, parsePackageManifestFile, parseWorkspace } from '../src/index';

const root = path.resolve(process.cwd(), '..');

function fixture(...parts: string[]): string {
  return path.join(root, 'fixtures', ...parts, 'manifest.tsx');
}

function golden(...parts: string[]): string {
  return fs.readFileSync(path.join(root, 'fixtures', ...parts, 'manifest.ir.golden.json'), 'utf8').trim();
}

describe('manifest frontend parser', () => {
  it('parses minimal-library', () => {
    const result = parseManifestFile(fixture('valid', 'minimal-library'));
    expect(result.ok).toBe(true);
    expect(JSON.stringify(result.ir)).toBe(golden('valid', 'minimal-library'));
  });

  it('parses machinalayout-like', () => {
    const result = parseManifestFile(fixture('valid', 'machinalayout-like'));
    expect(result.ok).toBe(true);
    expect(JSON.stringify(result.ir)).toBe(golden('valid', 'machinalayout-like'));
  });

  it('parses git-dep', () => {
    const result = parseManifestFile(fixture('valid', 'git-dep'));
    expect(result.ok).toBe(true);
    expect(JSON.stringify(result.ir)).toBe(golden('valid', 'git-dep'));
  });

  it('parses run targets', () => {
    const result = parseManifestFile(fixture('valid', 'm22-run-target'));
    expect(result.ok).toBe(true);
    expect(JSON.stringify(result.ir)).toBe(golden('valid', 'm22-run-target'));
  });


  it.each(['nodejs', 'bun', 'deno'] as const)('parses workspace runtime profile %s', (runtime) => {
    const tmpDir = fs.mkdtempSync(path.join(root, 'fixtures', `tmp-runtime-${runtime}-`));
    const manifestPath = path.join(tmpDir, 'manifest.tsx');
    fs.writeFileSync(
      manifestPath,
      `import { define } from "tspack/manifest";
export default define(
  <Workspace name="ws" runtime="${runtime}">
    <Package name="app" version="1.0.0" kind="app">
      <Targets rows={[{ name: "web", entry: "src/main.ts", runtime: "dist/main.js", types: "" }]} />
    </Package>
  </Workspace>
);
`,
      'utf8',
    );
    try {
      const result = parseManifestFile(manifestPath);
      expect(result.ok).toBe(true);
      expect(result.ir?.workspace.runtime).toBe(runtime);
    } finally {
      fs.rmSync(tmpDir, { recursive: true, force: true });
    }
  });

  it('defaults omitted workspace runtime profile to nodejs', () => {
    const result = parseManifestFile(fixture('valid', 'minimal-library'));
    expect(result.ok).toBe(true);
    expect(result.ir?.workspace.runtime).toBe('nodejs');
  });

  it('rejects invalid workspace runtime profiles', () => {
    const tmpDir = fs.mkdtempSync(path.join(root, 'fixtures', 'tmp-invalid-runtime-'));
    const manifestPath = path.join(tmpDir, 'manifest.tsx');
    fs.writeFileSync(
      manifestPath,
      `import { define } from "tspack/manifest";
export default define(
  <Workspace name="ws" runtime="npm">
    <Package name="app" version="1.0.0" kind="app">
      <Targets rows={[{ name: "web", entry: "src/main.ts", runtime: "dist/main.js", types: "" }]} />
    </Package>
  </Workspace>
);
`,
      'utf8',
    );
    try {
      const result = parseManifestFile(manifestPath);
      expect(result.ok).toBe(false);
      expect(result.diagnostics.some((d) => d.code === 'TSPACK_MANIFEST_INVALID_RUNTIME_PROFILE')).toBe(true);
    } finally {
      fs.rmSync(tmpDir, { recursive: true, force: true });
    }
  });

  it('workspace helper emits workspace source.name', () => {
    const tmpDir = fs.mkdtempSync(path.join(root, 'fixtures', 'tmp-workspace-helper-'));
    const manifestPath = path.join(tmpDir, 'manifest.tsx');
    fs.writeFileSync(
      manifestPath,
      `import { define, dep, defineDeps, workspace } from "tspack/manifest";
const deps = defineDeps({ core: dep(workspace("@acme/core")) });
export default define(
  <Workspace name="ws">
    <Package name="@acme/app" version="1.0.0" kind="library" dependencies={{ values: [deps.core] }}>
      <Targets rows={[{ name: "core", export: ".", entry: "src/index.ts", runtime: "dist/index.js", types: "dist/index.d.ts" }]} />
    </Package>
  </Workspace>
);
`,
      'utf8',
    );
    try {
      const result = parseManifestFile(manifestPath);
      expect(result.ok).toBe(true);
      const pkg = result.ir.packages[0];
      const depSource = pkg.dependencies[0].source as Record<string, unknown>;
      expect(depSource.kind).toBe('workspace');
      expect(depSource.name).toBe('@acme/core');
      expect(depSource.source).toBeUndefined();
    } finally {
      fs.rmSync(tmpDir, { recursive: true, force: true });
    }
  });

  it('accepts allowOnly boundary rows', () => {
    const tmpDir = fs.mkdtempSync(path.join(root, 'fixtures', 'tmp-allow-only-'));
    const manifestPath = path.join(tmpDir, 'manifest.tsx');
    fs.writeFileSync(
      manifestPath,
      `import { define } from "tspack/manifest";
export default define(
  <Workspace name="ws">
    <Package name="app" version="1.0.0" kind="library">
      <Targets rows={[{ name: "core", export: ".", entry: "src/index.ts", runtime: "dist/index.js", types: "dist/index.d.ts" }]} />
      <Boundaries rows={[{ from: "src/**", allowOnly: ["react"] }, { transitiveFrom: "src/index.ts", allowOnly: [] }]} />
      <Publish include={["dist/**"]} />
    </Package>
  </Workspace>
);
`,
      'utf8',
    );
    try {
      const result = parseManifestFile(manifestPath);
      expect(result.ok).toBe(true);
      const boundaries = result.ir.packages[0].boundaries as Array<Record<string, unknown>>;
      expect(boundaries[0].allowOnly).toEqual(['react']);
      expect(boundaries[1].allowOnly).toEqual([]);
    } finally {
      fs.rmSync(tmpDir, { recursive: true, force: true });
    }
  });


  it('accepts denyTypeDeps boundary rows', () => {
    const tmpDir = fs.mkdtempSync(path.join(root, 'fixtures', 'tmp-deny-type-deps-'));
    const manifestPath = path.join(tmpDir, 'manifest.tsx');
    fs.writeFileSync(
      manifestPath,
      `import { define } from "tspack/manifest";
export default define(
  <Workspace name="ws">
    <Package name="app" version="1.0.0" kind="library">
      <Targets rows={[{ name: "core", export: ".", entry: "src/index.ts", runtime: "dist/index.js", types: "dist/index.d.ts" }]} />
      <Boundaries rows={[{ from: "src/index.ts", denyTypeDeps: ["@internal/types"] }, { transitiveFrom: "src/index.ts", denyTypeDeps: [] }]} />
      <Publish include={["dist/**"]} />
    </Package>
  </Workspace>
);
`,
      'utf8',
    );
    try {
      const result = parseManifestFile(manifestPath);
      expect(result.ok).toBe(true);
      const boundaries = result.ir.packages[0].boundaries as Array<Record<string, unknown>>;
      expect(boundaries[0].denyTypeDeps).toEqual(['@internal/types']);
      expect(boundaries[1].denyTypeDeps).toEqual([]);
    } finally {
      fs.rmSync(tmpDir, { recursive: true, force: true });
    }
  });

  it('parses lifecycle capability acknowledgements from Security', () => {
    const tmpDir = fs.mkdtempSync(path.join(root, 'fixtures', 'tmp-security-'));
    const manifestPath = path.join(tmpDir, 'manifest.tsx');
    fs.writeFileSync(
      manifestPath,
      `import { define } from "tspack/manifest";
export default define(
  <Workspace name="ws">
    <Security acknowledgedCapabilities={[{
      package: "npm:dep-a@1.0.0",
      kind: "lifecycleScript",
      script: "postinstall",
      command: "node install.js",
      reason: "Known lifecycle capability; execution remains blocked by TSPack.",
      behaviorFixture: "security/dep-a-postinstall.valid.xtest.tsx",
      behaviorReport: "security/dep-a-postinstall.report.json",
    }]} />
    <Package name="app" version="1.0.0" kind="library">
      <Targets rows={[{ name: "core", export: ".", entry: "src/index.ts", runtime: "dist/index.js", types: "dist/index.d.ts" }]} />
      <Publish include={["dist/**"]} />
    </Package>
  </Workspace>
);
`,
      'utf8',
    );
    try {
      const result = parseManifestFile(manifestPath);
      expect(result.ok).toBe(true);
      expect(result.ir?.security?.acknowledgedCapabilities).toEqual([
        {
          package: 'npm:dep-a@1.0.0',
          kind: 'lifecycleScript',
          script: 'postinstall',
          command: 'node install.js',
          reason: 'Known lifecycle capability; execution remains blocked by TSPack.',
          behaviorFixture: 'security/dep-a-postinstall.valid.xtest.tsx',
          behaviorReport: 'security/dep-a-postinstall.report.json',
        },
      ]);
    } finally {
      fs.rmSync(tmpDir, { recursive: true, force: true });
    }
  });

  it('run targets parse deterministically', () => {
    const a = JSON.stringify(parseManifestFile(fixture('valid', 'm22-run-target')).ir);
    const b = JSON.stringify(parseManifestFile(fixture('valid', 'm22-run-target')).ir);
    expect(a).toBe(b);
  });

  it('is deterministic', () => {
    const a = JSON.stringify(parseManifestFile(fixture('valid', 'minimal-library')).ir);
    const b = JSON.stringify(parseManifestFile(fixture('valid', 'minimal-library')).ir);
    expect(a).toBe(b);
  });

  it('parses package manifest files directly', () => {
    const manifestPath = path.join(root, 'fixtures', 'valid', 'm6b-workspace-split', 'packages', 'core', 'package.manifest.tsx');
    const result = parsePackageManifestFile(manifestPath);
    expect(result.ok).toBe(true);
    expect(result.ir?.packages[0]?.name).toBe('@m6b/core');
  });

  it('parseWorkspace parses split workspace deterministically', () => {
    const manifestPath = fixture('valid', 'm6b-workspace-split');
    const a = JSON.stringify(parseWorkspace(manifestPath).ir);
    const b = JSON.stringify(parseWorkspace(manifestPath).ir);
    expect(a).toBe(b);
    expect(parseWorkspace(manifestPath).ok).toBe(true);
  });

  it.each([
    ['forbidden-import', 'TSPACK_MANIFEST_FORBIDDEN_IMPORT'],
    ['process-env', 'TSPACK_MANIFEST_FORBIDDEN_PROCESS_ENV'],
    ['dynamic-manifest-map', 'TSPACK_MANIFEST_FORBIDDEN_DYNAMIC_EXPRESSION'],
    ['forbidden-function', 'TSPACK_MANIFEST_FORBIDDEN_FUNCTION'],
    ['unknown-element', 'TSPACK_MANIFEST_UNKNOWN_ELEMENT'],
    ['unknown-helper', 'TSPACK_MANIFEST_UNKNOWN_HELPER'],
    ['spread-object', 'TSPACK_MANIFEST_FORBIDDEN_SPREAD'],
    ['spread-array', 'TSPACK_MANIFEST_FORBIDDEN_SPREAD'],
    ['spread-jsx-props', 'TSPACK_MANIFEST_FORBIDDEN_SPREAD'],
  ])('fails %s with %s', (name, code) => {
    const result = parseManifestFile(fixture('invalid', name));
    expect(result.ok).toBe(false);
    expect(result.diagnostics.some((d) => d.code === code)).toBe(true);
  });

  it('non-root manifest fails', () => {
    const tmp = path.join(root, 'fixtures', 'valid', 'minimal-library', 'not-root.tsx');
    fs.copyFileSync(fixture('valid', 'minimal-library'), tmp);
    const result = parseManifestFile(tmp);
    fs.unlinkSync(tmp);
    expect(result.diagnostics.some((d) => d.code === 'TSPACK_MANIFEST_NON_ROOT')).toBe(true);
  });

  it('diagnostics sorted deterministically', () => {
    const result = parseManifestFile(fixture('invalid', 'dynamic-manifest-map'));
    const sorted = [...result.diagnostics].sort((a, b) => `${a.file}:${a.line ?? 0}:${a.column ?? 0}:${a.code}:${a.message}`.localeCompare(`${b.file}:${b.line ?? 0}:${b.column ?? 0}:${b.code}:${b.message}`));
    expect(result.diagnostics).toEqual(sorted);
  });
});
