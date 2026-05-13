import { createNativeArtifactReport, createNativeBenchmarkReport, createNativeTestReport, formatNativeArtifactJsonReport, formatNativeArtifactTextReport, formatNativeBenchmarkJsonReport, formatNativeBenchmarkTextReport, formatNativeTestJsonReport, formatNativeTestTextReport, listNativeArtifacts, listNativeBenchmarks, listNativeTests, nativeArtifactExitCode, nativeBenchmarkExitCode, nativeTestExitCode, runNativeArtifacts, runNativeBenchmarks, runNativeTestFiles } from './native-test/index.js';

type Mode = 'test' | 'artifact' | 'bench';
type Options = { mode: Mode; rootDir: string; list: boolean; filter?: string; json: boolean; out?: string };

function parseArgs(argv: string[]): Options {
  const mode = (argv[2] === 'artifact' ? 'artifact' : argv[2] === 'bench' ? 'bench' : 'test') as Mode;
  const options: Options = { mode, rootDir: '.', list: false, json: false };
  for (let i = 3; i < argv.length; i += 1) {
    const arg = argv[i];
    if (arg === '--root') { i += 1; options.rootDir = argv[i] ?? '.'; continue; }
    if (arg === '--list') { options.list = true; continue; }
    if (arg === '--filter') { i += 1; options.filter = argv[i]; continue; }
    if (arg === '--json') { options.json = true; continue; }
    if (arg === '--out') { i += 1; options.out = argv[i]; continue; }
    throw new Error(`unknown flag: ${arg}`);
  }
  return options;
}

async function main(): Promise<void> {
  const options = parseArgs(process.argv);
  if (options.mode === 'artifact') {
    if (options.list) {
      const listed = await listNativeArtifacts({ rootDir: options.rootDir });
      const report = createNativeArtifactReport({ artifacts: listed.artifacts.map((a) => ({ id: a.id, name: a.name, status: 'passed' })), diagnostics: listed.diagnostics });
      process.stdout.write(options.json ? formatNativeArtifactJsonReport(report) : formatNativeArtifactTextReport(report));
      process.exit(nativeArtifactExitCode(report));
    }
    const result = await runNativeArtifacts({ rootDir: options.rootDir, filter: options.filter, artifactRoot: options.out });
    const report = createNativeArtifactReport(result);
    process.stdout.write(options.json ? formatNativeArtifactJsonReport(report) : formatNativeArtifactTextReport(report));
    process.exit(nativeArtifactExitCode(report));
  }
  if (options.mode === 'bench') {
    if (options.list) {
      const listed = listNativeBenchmarks({ rootDir: options.rootDir });
      const report = createNativeBenchmarkReport({ benchmarks: listed.benchmarks.map((b) => ({ id: b.id, name: b.name, status: 'passed', iterations: b.iterations, warmup: b.warmup })), diagnostics: listed.diagnostics });
      process.stdout.write(options.json ? formatNativeBenchmarkJsonReport(report) : formatNativeBenchmarkTextReport(report));
      process.exit(nativeBenchmarkExitCode(report));
    }
    const run = await runNativeBenchmarks({ rootDir: options.rootDir, filter: options.filter });
    const report = createNativeBenchmarkReport(run);
    process.stdout.write(options.json ? formatNativeBenchmarkJsonReport(report) : formatNativeBenchmarkTextReport(report));
    process.exit(nativeBenchmarkExitCode(report));
  }

  if (options.list) {
    const listed = await listNativeTests({ rootDir: options.rootDir });
    const report = createNativeTestReport({ results: listed.tests.map((t) => ({ id: t.id, name: t.name, status: 'passed' })), diagnostics: listed.diagnostics });
    process.stdout.write(options.json ? formatNativeTestJsonReport(report) : formatNativeTestTextReport(report));
    process.exit(nativeTestExitCode(report));
  }
  const runResult = await runNativeTestFiles({ rootDir: options.rootDir, filter: options.filter });
  const report = createNativeTestReport(runResult);
  process.stdout.write(options.json ? formatNativeTestJsonReport(report) : formatNativeTestTextReport(report));
  process.exit(nativeTestExitCode(report));
}

main().catch((error: unknown) => {
  process.stderr.write(`TSPACK_TEST_XTEST_FAILED: ${String(error)}\n`);
  process.exit(1);
});
