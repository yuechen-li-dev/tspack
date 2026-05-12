import fs from 'node:fs';
import path from 'node:path';
import { describe, expect, it } from 'vitest';
import { parseManifestFile, parseWorkspace } from '../src/index';

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

  it('is deterministic', () => {
    const a = JSON.stringify(parseManifestFile(fixture('valid', 'minimal-library')).ir);
    const b = JSON.stringify(parseManifestFile(fixture('valid', 'minimal-library')).ir);
    expect(a).toBe(b);
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
