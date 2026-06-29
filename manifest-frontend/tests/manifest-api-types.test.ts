import { execFileSync } from 'node:child_process';
import path from 'node:path';
import { describe, expect, it } from 'vitest';

function runTypecheck(tsconfigName: string): { status: number; output: string } {
  const manifestFrontendDir = path.resolve(__dirname, '..');
  const tsconfigPath = path.join(manifestFrontendDir, 'tests', 'typing-fixtures', tsconfigName);
  const tscPath = path.join(
    manifestFrontendDir,
    'node_modules',
    'typescript',
    'bin',
    'tsc',
  );

  try {
    execFileSync(process.execPath, [tscPath, '-p', tsconfigPath, '--noEmit'], {
      cwd: manifestFrontendDir,
      stdio: 'pipe',
      encoding: 'utf8',
    });
    return { status: 0, output: '' };
  } catch (error) {
    const output =
      error instanceof Error && 'stdout' in error
        ? `${String((error as { stdout?: string }).stdout ?? '')}${String((error as { stderr?: string }).stderr ?? '')}`
        : String(error);
    const status =
      typeof (error as { status?: unknown }).status === 'number'
        ? ((error as { status: number }).status ?? 1)
        : 1;
    return { status, output };
  }
}

describe('manifest API type surface', () => {
  it('valid fixtures typecheck successfully', () => {
    const result = runTypecheck('tsconfig.valid.json');
    expect(result.status).toBe(0);
  });

  it('invalid fixtures fail typecheck with expected errors', () => {
    const result = runTypecheck('tsconfig.invalid.json');
    expect(result.status).toBe(2);
    expect(result.output).toContain(`Type '"npm"' is not assignable to type '"bun" | "deno" | "system" | "node" | undefined'`);
    expect(result.output).toContain("missing the following properties from type 'PackageProps': name, version");
    expect(result.output).toContain("missing the following properties from type 'TargetRow': entry, runtime");
    expect(result.output).toContain("Type 'number' is not assignable to type 'string'");
    expect(result.output).toContain("Property 'path' is missing");
    expect(result.output).toContain("Property 'value' is missing");
    expect(result.output).toContain("Type '() => string' is not assignable to type 'JSONValue'");
  });
});
