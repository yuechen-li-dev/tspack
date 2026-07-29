import fs from 'node:fs';
import os from 'node:os';
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

function withTemporaryManifest(
  prefix: string,
  contents: string,
  assertion: (manifestPath: string) => void,
): void {
  const tmpDir = fs.mkdtempSync(path.join(root, 'fixtures', prefix));
  const manifestPath = path.join(tmpDir, 'manifest.tsx');
  fs.writeFileSync(manifestPath, contents, 'utf8');

  try {
    assertion(manifestPath);
  } finally {
    fs.rmSync(tmpDir, { recursive: true, force: true });
  }
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

  it('treats omitted runtime and explicit nodejs as equivalent baseline manifests', () => {
    const omitted = parseManifestFile(fixture('valid', 'runtime-baseline-omitted'));
    const explicit = parseManifestFile(fixture('valid', 'runtime-baseline-nodejs'));

    expect(omitted.ok).toBe(true);
    expect(explicit.ok).toBe(true);
    expect(omitted.ir?.workspace.runtime).toBe('nodejs');
    expect(explicit.ir?.workspace.runtime).toBe('nodejs');
    expect(omitted.ir).toEqual(explicit.ir);
    expect(JSON.stringify(omitted.ir)).toBe(golden('valid', 'runtime-baseline-omitted'));
    expect(JSON.stringify(explicit.ir)).toBe(golden('valid', 'runtime-baseline-nodejs'));
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


  it('uses explicit dependency key before defineDeps alias for scoped tool refs', () => {
    withTemporaryManifest(
      'tmp-scoped-tool-explicit-key-',
      `import { define, defineDeps, npm, tool } from "tspack/manifest";
const deps = defineDeps({
  biome: tool(npm("@biomejs/biome", "^1.9.4"), {
    key: "@biomejs/biome",
  }),
});
export default define(
  <Workspace name="ws">
    <Package name="app" version="1.0.0" kind="library" dependencies={{ values: [deps.biome] }}>
      <Targets rows={[{ name: "core", export: ".", entry: "src/index.ts", runtime: "dist/index.js", types: "dist/index.d.ts" }]} />
      <Tools values={[deps.biome]} />
    </Package>
  </Workspace>
);
`,
      (manifestPath) => {
        const result = parseManifestFile(manifestPath);
        expect(result.ok).toBe(true);
        const pkg = result.ir?.packages[0] as Record<string, any>;
        expect(pkg.dependencies[0].key).toBe('@biomejs/biome');
        expect(pkg.tools).toEqual(['@biomejs/biome']);
      },
    );
  });

  it('preserves defineDeps property alias fallback for unscoped tool refs', () => {
    withTemporaryManifest(
      'tmp-tool-alias-fallback-',
      `import { define, defineDeps, npm, tool } from "tspack/manifest";
const deps = defineDeps({
  typescript: tool(npm("typescript", "^5.0.0")),
});
export default define(
  <Workspace name="ws">
    <Package name="app" version="1.0.0" kind="library" dependencies={{ values: [deps.typescript] }}>
      <Targets rows={[{ name: "core", export: ".", entry: "src/index.ts", runtime: "dist/index.js", types: "dist/index.d.ts" }]} />
      <Tools values={[deps.typescript]} />
    </Package>
  </Workspace>
);
`,
      (manifestPath) => {
        const result = parseManifestFile(manifestPath);
        expect(result.ok).toBe(true);
        const pkg = result.ir?.packages[0] as Record<string, any>;
        expect(pkg.dependencies[0].key).toBeUndefined();
        expect(pkg.tools).toEqual(['typescript']);
      },
    );
  });

  it('keeps scoped package alias fallback when no explicit key is provided', () => {
    withTemporaryManifest(
      'tmp-scoped-tool-alias-fallback-',
      `import { define, defineDeps, npm, tool } from "tspack/manifest";
const deps = defineDeps({
  biome: tool(npm("@biomejs/biome", "^1.9.4")),
});
export default define(
  <Workspace name="ws">
    <Package name="app" version="1.0.0" kind="library" dependencies={{ values: [deps.biome] }}>
      <Targets rows={[{ name: "core", export: ".", entry: "src/index.ts", runtime: "dist/index.js", types: "dist/index.d.ts" }]} />
      <Tools values={[deps.biome]} />
    </Package>
  </Workspace>
);
`,
      (manifestPath) => {
        const result = parseManifestFile(manifestPath);
        expect(result.ok).toBe(true);
        const pkg = result.ir?.packages[0] as Record<string, any>;
        expect(pkg.dependencies[0].key).toBeUndefined();
        expect(pkg.tools).toEqual(['biome']);
      },
    );
  });

  it('uses explicit dependency key before defineDeps alias for scoped target refs', () => {
    withTemporaryManifest(
      'tmp-scoped-target-explicit-key-',
      `import { define, dep, defineDeps, npm } from "tspack/manifest";
const deps = defineDeps({
  reactTypes: dep(npm("@types/react", "^18.0.0"), {
    key: "@types/react",
  }),
});
export default define(
  <Workspace name="ws">
    <Package name="app" version="1.0.0" kind="library" dependencies={{ values: [deps.reactTypes] }}>
      <Targets rows={[{ name: "core", export: ".", entry: "src/index.ts", runtime: "dist/index.js", types: "dist/index.d.ts", deps: [deps.reactTypes] }]} />
    </Package>
  </Workspace>
);
`,
      (manifestPath) => {
        const result = parseManifestFile(manifestPath);
        expect(result.ok).toBe(true);
        const pkg = result.ir?.packages[0] as Record<string, any>;
        expect(pkg.dependencies[0].key).toBe('@types/react');
        expect(pkg.targets[0].deps).toEqual(['@types/react']);
      },
    );
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
    <Security
      acknowledgedCapabilities={[{
        package: "npm:dep-a@1.0.0",
        kind: "lifecycleScript",
        script: "postinstall",
        command: "node install.js",
        reason: "Known lifecycle capability; execution remains blocked by TSPack.",
        behaviorFixture: "security/dep-a-postinstall.valid.xtest.tsx",
        behaviorReport: "security/dep-a-postinstall.report.json",
      }]}
      acknowledgedLifecycleCategories={[{
        category: "maintainer-publish",
        scripts: ["prepare"],
        reason: "Maintainer lifecycle scripts remain blocked by TSPack.",
      }]}
    />
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
      expect(result.ir?.security?.acknowledgedLifecycleCategories).toEqual([
        {
          category: 'maintainer-publish',
          scripts: ['prepare'],
          reason: 'Maintainer lifecycle scripts remain blocked by TSPack.',
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

  it('parseWorkspace preserves root Security in split manifests', () => {
    const tmpDir = fs.mkdtempSync(path.join(root, 'fixtures', 'tmp-split-security-'));
    const packageDir = path.join(tmpDir, 'packages', 'app');
    fs.mkdirSync(packageDir, { recursive: true });
    const manifestPath = path.join(tmpDir, 'manifest.tsx');

    fs.writeFileSync(
      manifestPath,
      `import { defineWorkspace } from "tspack/manifest";
export default defineWorkspace(
  <Workspace name="ws">
    <Packages rows={[{ name: "app", root: "packages/app", manifest: "packages/app/package.manifest.tsx" }]} />
    <Security acknowledgedCapabilities={[{
      package: "npm:dep-a@1.0.0",
      kind: "lifecycleScript",
      script: "postinstall",
      command: "node install.js",
      reason: "Known lifecycle capability; execution remains blocked by TSPack.",
      behaviorFixture: "security/dep-a-postinstall.valid.xtest.tsx",
    }]} />
  </Workspace>
);
`,
      'utf8',
    );
    fs.writeFileSync(
      path.join(packageDir, 'package.manifest.tsx'),
      `import { definePackage } from "tspack/manifest";
export default definePackage(
  <Package name="app" version="1.0.0" kind="library">
    <Targets rows={[{ name: "core", export: ".", entry: "src/index.ts", runtime: "dist/index.js", types: "dist/index.d.ts" }]} />
    <Publish include={["dist/**"]} />
  </Package>
);
`,
      'utf8',
    );

    try {
      const result = parseWorkspace(manifestPath);
      expect(result.ok).toBe(true);
      expect(result.ir?.packages).toHaveLength(1);
      expect(result.ir?.security?.acknowledgedCapabilities).toEqual([
        {
          package: 'npm:dep-a@1.0.0',
          kind: 'lifecycleScript',
          script: 'postinstall',
          command: 'node install.js',
          reason: 'Known lifecycle capability; execution remains blocked by TSPack.',
          behaviorFixture: 'security/dep-a-postinstall.valid.xtest.tsx',
        },
      ]);
    } finally {
      fs.rmSync(tmpDir, { recursive: true, force: true });
    }
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
  it('lowers typed JSON compat helper presets through compatFiles IR', () => {
    withTemporaryManifest(
      'tmp-compat-json-helpers-',
      `import { TsConfig, VSCode, defineWorkspace } from "tspack/manifest";
export default defineWorkspace(
  <Workspace name="compat-json-helpers">
    <CompatFiles>
      <JsonFile path="tsconfig.tspack.json" value={TsConfig.manifestEditor()} />
      <JsonFile path=".vscode/settings.json" value={VSCode.settings({ "typescript.tsdk": "node_modules/typescript/lib" })} />
      <JsonFile path=".vscode/extensions.json" value={VSCode.extensions({ recommendations: ["biomejs.biome"] })} />
    </CompatFiles>
  </Workspace>
);`,
      (manifestPath) => {
        const result = parseManifestFile(manifestPath);
        expect(result.ok).toBe(true);
        expect(result.ir?.compatFiles).toEqual([
          {
            format: 'json',
            path: 'tsconfig.tspack.json',
            value: expect.objectContaining({
              compilerOptions: expect.objectContaining({
                jsx: 'preserve',
                moduleResolution: 'Bundler',
                types: [],
                baseUrl: '.',
                ignoreDeprecations: '5.0',
              }),
              include: expect.arrayContaining(['manifest.tsx', '**/*.xtest.tsx']),
              exclude: expect.not.arrayContaining(['src/**']),
            }),
          },
          {
            format: 'json',
            path: '.vscode/settings.json',
            value: {
              'typescript.enablePromptUseWorkspaceTsdk': true,
              'typescript.tsdk': 'node_modules/typescript/lib',
            },
          },
          {
            format: 'json',
            path: '.vscode/extensions.json',
            value: { recommendations: ['biomejs.biome'] },
          },
        ]);
      },
    );
  });

  it('preserves an explicit tscl compiler selection without changing the Node runtime', () => {
    const result = parseManifestFile(path.join(root, 'fixtures', 'tscl-m1', 'manifest.tsx'));
    expect(result.ok).toBe(true);
    expect(result.ir?.workspace.runtime).toBe('nodejs');
    expect(result.ir?.packages[0]?.compiler).toBe('tscl');
  });

  it('uses canonical manifest editor defaults when no overrides are supplied', () => {
    withTemporaryManifest(
      'tmp-compat-json-helper-defaults-',
      `import { TsConfig, defineWorkspace } from "tspack/manifest";
export default defineWorkspace(
  <Workspace name="compat-json-helpers">
    <CompatFiles>
      <JsonFile path="tsconfig.tspack.json" value={TsConfig.manifestEditor()} />
    </CompatFiles>
  </Workspace>
);`,
      (manifestPath) => {
        const result = parseManifestFile(manifestPath);
        expect(result.ok).toBe(true);
        const tsconfig = result.ir?.compatFiles?.[0]?.value as Record<string, unknown>;
        expect(tsconfig).toEqual({
          compilerOptions: {
            target: 'ES2022',
            module: 'ESNext',
            moduleResolution: 'Bundler',
            jsx: 'preserve',
            strict: true,
            noEmit: true,
            types: [],
            baseUrl: '.',
            ignoreDeprecations: '5.0',
            paths: {
              'tspack/manifest': ['.tspack/types/tspack-manifest.d.ts'],
            },
          },
          include: [
            'manifest.tsx',
            'package.manifest.tsx',
            '**/*.manifest.tsx',
            '**/*.xtest.tsx',
            '.tspack/types/**/*.d.ts',
          ],
          exclude: [
            'dist/**',
            'node_modules/**',
            '.tspack/store/**',
            'tspack-artifacts/**',
          ],
        });
      },
    );
  });

  it('uses exact manifest editor include and exclude overrides when provided', () => {
    withTemporaryManifest(
      'tmp-compat-json-helper-overrides-',
      `import { TsConfig, defineWorkspace } from "tspack/manifest";
export default defineWorkspace(
  <Workspace name="compat-json-helpers">
    <CompatFiles>
      <JsonFile
        path="tsconfig.tspack.json"
        value={TsConfig.manifestEditor({
          include: ["manifest.tsx", "examples/demo/manifest.tsx"],
          exclude: ["dist/**", "fixtures/**"],
        })}
      />
    </CompatFiles>
  </Workspace>
);`,
      (manifestPath) => {
        const result = parseManifestFile(manifestPath);
        expect(result.ok).toBe(true);
        const tsconfig = result.ir?.compatFiles?.[0]?.value as Record<string, unknown>;
        expect(tsconfig).toMatchObject({
          compilerOptions: {
            target: 'ES2022',
            moduleResolution: 'Bundler',
            jsx: 'preserve',
            types: [],
            baseUrl: '.',
            ignoreDeprecations: '5.0',
            paths: {
              'tspack/manifest': ['.tspack/types/tspack-manifest.d.ts'],
            },
          },
          include: ['manifest.tsx', 'examples/demo/manifest.tsx'],
          exclude: ['dist/**', 'fixtures/**'],
        });
        expect(tsconfig.include).not.toContain('.tspack/types/**/*.d.ts');
      },
    );
  });

  it.each([
    ['include must be an array', `TsConfig.manifestEditor({ include: 123 })`],
    ['exclude must be an array', `TsConfig.manifestEditor({ exclude: "fixtures/**" })`],
    ['include entries must be strings', `TsConfig.manifestEditor({ include: ["manifest.tsx", 123] })`],
    ['exclude entries reject empty strings', `TsConfig.manifestEditor({ exclude: [""] })`],
    ['include entries reject absolute paths', `TsConfig.manifestEditor({ include: ["C:/repo/manifest.tsx"] })`],
    ['include entries reject traversal', `TsConfig.manifestEditor({ include: ["../manifest.tsx"] })`],
    ['exclude entries reject backslashes', `TsConfig.manifestEditor({ exclude: ["fixtures\\\\**"] })`],
  ])('rejects invalid manifest editor options: %s', (_name, helperCall) => {
    withTemporaryManifest(
      'tmp-compat-json-helper-invalid-',
      `import { TsConfig, defineWorkspace } from "tspack/manifest";
export default defineWorkspace(
  <Workspace name="compat-json-helpers">
    <CompatFiles>
      <JsonFile path="tsconfig.tspack.json" value={${helperCall}} />
    </CompatFiles>
  </Workspace>
);`,
      (manifestPath) => {
        const result = parseManifestFile(manifestPath);
        expect(result.ok).toBe(false);
        expect(result.diagnostics.some((d) => d.code === 'TSPACK_MANIFEST_INVALID_HELPER_ARGUMENT')).toBe(true);
      },
    );
  });

  it('uses workspace TypeScript settings defaults and merges additional settings', () => {
    withTemporaryManifest(
      'tmp-vscode-settings-defaults-',
      `import { VSCode, defineWorkspace } from "tspack/manifest";
export default defineWorkspace(
  <Workspace name="compat-json-helpers">
    <CompatFiles>
      <JsonFile
        path=".vscode/settings.json"
        value={VSCode.settings({
          "editor.defaultFormatter": "biomejs.biome",
        })}
      />
    </CompatFiles>
  </Workspace>
);`,
      (manifestPath) => {
        const result = parseManifestFile(manifestPath);
        expect(result.ok).toBe(true);
        expect(result.ir?.compatFiles?.[0]?.value).toEqual({
          'editor.defaultFormatter': 'biomejs.biome',
          'typescript.enablePromptUseWorkspaceTsdk': true,
          'typescript.tsdk': 'node_modules/typescript/lib',
        });
      },
    );
  });

  it('supports a custom relative workspace TypeScript SDK path', () => {
    withTemporaryManifest(
      'tmp-vscode-settings-custom-tsdk-',
      `import { VSCode, defineWorkspace } from "tspack/manifest";
export default defineWorkspace(
  <Workspace name="compat-json-helpers">
    <CompatFiles>
      <JsonFile
        path=".vscode/settings.json"
        value={VSCode.settings({
          typescriptTsdk: "manifest-frontend/node_modules/typescript/lib",
        })}
      />
    </CompatFiles>
  </Workspace>
);`,
      (manifestPath) => {
        const result = parseManifestFile(manifestPath);
        expect(result.ok).toBe(true);
        expect(result.ir?.compatFiles?.[0]?.value).toEqual({
          'typescript.enablePromptUseWorkspaceTsdk': true,
          'typescript.tsdk': 'manifest-frontend/node_modules/typescript/lib',
        });
      },
    );
  });

  it.each([
    ['empty string', `VSCode.settings({ typescriptTsdk: "" })`],
    ['absolute path', `VSCode.settings({ typescriptTsdk: "/tmp/typescript/lib" })`],
    ['traversal', `VSCode.settings({ typescriptTsdk: "../node_modules/typescript/lib" })`],
    ['backslashes', `VSCode.settings({ typescriptTsdk: "node_modules\\\\typescript\\\\lib" })`],
    ['url', `VSCode.settings({ typescriptTsdk: "https://example.com/typescript/lib" })`],
    ['drive letter', `VSCode.settings({ typescriptTsdk: "C:/typescript/lib" })`],
  ])('rejects invalid VS Code TypeScript SDK paths: %s', (_name, helperCall) => {
    withTemporaryManifest(
      'tmp-vscode-settings-invalid-tsdk-',
      `import { VSCode, defineWorkspace } from "tspack/manifest";
export default defineWorkspace(
  <Workspace name="compat-json-helpers">
    <CompatFiles>
      <JsonFile path=".vscode/settings.json" value={${helperCall}} />
    </CompatFiles>
  </Workspace>
);`,
      (manifestPath) => {
        const result = parseManifestFile(manifestPath);
        expect(result.ok).toBe(false);
        expect(result.diagnostics.some((d) => d.code === 'TSPACK_MANIFEST_INVALID_HELPER_ARGUMENT')).toBe(true);
      },
    );
  });

});

  it('parses UpdatePolicy in root manifests', () => {
    const dir = fs.mkdtempSync(path.join(os.tmpdir(), 'tspack-policy-'));
    const manifestPath = path.join(dir, 'manifest.tsx');
    fs.writeFileSync(
      manifestPath,
      `import { define, Workspace, Package, UpdatePolicy } from "tspack/manifest";
export default define(
  <Workspace name="ws">
    <UpdatePolicy rows={[{ name: "typescript", kind: "tool", strategy: "rolling", level: "minor", reason: "tooling can roll" }]} />
    <Package name="app" version="1.0.0" kind="library" dependencies={{ values: [] }} />
  </Workspace>
);`
    );
    const result = parseManifestFile(manifestPath);
    expect(result.ok).toBe(true);
    expect((result.ir as any).updatePolicy.rows[0].name).toBe('typescript');
  });

  it('parseWorkspace preserves root UpdatePolicy in split manifests', () => {
    const dir = fs.mkdtempSync(path.join(os.tmpdir(), 'tspack-policy-split-'));
    fs.mkdirSync(path.join(dir, 'packages', 'app'), { recursive: true });
    const manifestPath = path.join(dir, 'manifest.tsx');
    fs.writeFileSync(
      manifestPath,
      `import { defineWorkspace, Workspace, Packages, UpdatePolicy } from "tspack/manifest";
export default defineWorkspace(
  <Workspace name="ws">
    <UpdatePolicy rows={[{ name: "react", kind: "dep", strategy: "manual" }]} />
    <Packages rows={[{ name: "app", root: "packages/app", manifest: "packages/app/package.manifest.tsx" }]} />
  </Workspace>
);`
    );
    fs.writeFileSync(
      path.join(dir, 'packages', 'app', 'package.manifest.tsx'),
      `import { definePackage, Package } from "tspack/manifest";
export default definePackage(<Package name="app" version="1.0.0" kind="library" dependencies={{ values: [] }} />);`
    );
    const result = parseWorkspace(manifestPath);
    expect(result.ok).toBe(true);
    expect((result.ir as any).updatePolicy.rows[0].strategy).toBe('manual');
  });
