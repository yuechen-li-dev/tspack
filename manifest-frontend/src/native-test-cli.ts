import { createNativeTestReport, formatNativeTestJsonReport, formatNativeTestTextReport, listNativeTests, nativeTestExitCode, runNativeTestFiles } from './native-test/index.js';

type Options = {
  rootDir: string;
  list: boolean;
  filter?: string;
  json: boolean;
};

function parseArgs(argv: string[]): Options {
  const options: Options = { rootDir: '.', list: false, json: false };
  for (let index = 2; index < argv.length; index += 1) {
    const arg = argv[index];
    if (arg === 'test') {
      continue;
    }
    if (arg === '--root') {
      index += 1;
      options.rootDir = argv[index] ?? '.';
      continue;
    }
    if (arg === '--list') {
      options.list = true;
      continue;
    }
    if (arg === '--filter') {
      index += 1;
      options.filter = argv[index];
      continue;
    }
    if (arg === '--json') {
      options.json = true;
      continue;
    }
    throw new Error(`unknown flag: ${arg}`);
  }
  return options;
}

async function main(): Promise<void> {
  const options = parseArgs(process.argv);
  if (options.list) {
    const listed = await listNativeTests({ rootDir: options.rootDir, filter: options.filter });
    const report = createNativeTestReport({
      rootDir: listed.rootDir,
      discovered: listed.discovered,
      results: [],
      diagnostics: listed.diagnostics,
    });
    process.stdout.write(`${formatNativeTestTextReport(report)}\n`);
    process.exit(nativeTestExitCode(report));
  }

  const runResult = await runNativeTestFiles({ rootDir: options.rootDir, filter: options.filter });
  const report = createNativeTestReport(runResult);
  if (options.json) {
    process.stdout.write(`${formatNativeTestJsonReport(report)}\n`);
  } else {
    process.stdout.write(`${formatNativeTestTextReport(report)}\n`);
  }
  process.exit(nativeTestExitCode(report));
}

main().catch((error: unknown) => {
  process.stderr.write(`TSPACK_TEST_XTEST_FAILED: ${String(error)}\n`);
  process.exit(1);
});
