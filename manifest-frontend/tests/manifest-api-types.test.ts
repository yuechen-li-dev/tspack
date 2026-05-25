import { execFileSync } from 'node:child_process';
import path from 'node:path';
import { describe, expect, it } from 'vitest';

function runTypecheck(tsconfigName: string): { status: number; output: string } {
  const manifestFrontendDir = path.resolve(__dirname, '..');
  const tsconfigPath = path.join(manifestFrontendDir, 'tests', 'typing-fixtures', tsconfigName);

  try {
    execFileSync('npx', ['tsc', '-p', tsconfigPath, '--noEmit'], {
      cwd: manifestFrontendDir,
      stdio: 'pipe',
      encoding: 'utf8',
    });
    return { status: 0, output: '' };
  } catch (error) {
    const output = error instanceof Error && 'stdout' in error
      ? `${String((error as { stdout?: string }).stdout ?? '')}${String((error as { stderr?: string }).stderr ?? '')}`
      : String(error);
    return { status: 1, output };
  }
}

describe('manifest API type surface', () => {
  it('valid fixtures typecheck successfully', () => {
    const result = runTypecheck('tsconfig.valid.json');
    expect(result.status).toBe(0);
  });

  it('invalid fixtures fail typecheck with expected errors', () => {
    const result = runTypecheck('tsconfig.invalid.json');
    expect(result.status).toBe(1);
    expect(result.output).toContain(`Type '"bun"' is not assignable to type '"system" | "node"'`);
    expect(result.output).toContain("missing the following properties from type 'PackageProps': name, version");
    expect(result.output).toContain("missing the following properties from type 'TargetRow': entry, runtime");
  });
});
