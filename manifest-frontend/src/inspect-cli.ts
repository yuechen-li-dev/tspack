import { buildInspectFailureResult, inspectAndWrite, parsePoint, parseViewport } from './inspect/index.js';
import { formatInspectJson } from './inspect/format.js';
import type { InspectOptions } from './inspect/index.js';

function parseInspectCliArgs(args: string[]): InspectOptions {
  const cursor = args[0] === 'inspect' ? 1 : 0;
  let url: string | undefined;
  const options: Omit<InspectOptions, 'url'> = {
    browser: 'auto',
    viewport: { width: 1440, height: 900 },
    selector: undefined as string | undefined,
    points: [] as Array<{ x: number; y: number }>,
    json: false,
    out: undefined as string | undefined,
    text: undefined as string | undefined,
    root: undefined as string | undefined,
    bundle: false,
    bundleOutput: undefined as string | undefined,
    browserPath: undefined as string | undefined,
    hostPath: undefined as string | undefined,
    cdpEndpoint: undefined as string | undefined,
    listTargets: false,
    target: undefined as string | undefined,
    targetUrl: undefined as string | undefined
  };

  for (let i = cursor; i < args.length; i += 1) {
    const arg = args[i];
    if (!arg.startsWith('--') && !url) {
      url = arg;
      continue;
    }
    if (arg === '--url') { i += 1; url = args[i]; continue; }
    if (arg === '--json') { options.json = true; continue; }
    if (arg === '--out') { i += 1; options.out = args[i]; continue; }
    if (arg === '--text') { i += 1; options.text = args[i]; continue; }
    if (arg === '--root') {
      if (i + 1 >= args.length || args[i + 1].startsWith('--')) {
        throw new Error('TSPACK_INSPECT_INVALID_TARGET_OPTIONS: --root requires a value');
      }
      i += 1;
      options.root = args[i];
      continue;
    }
    if (arg === '--bundle') { options.bundle = true; continue; }
    if (arg === '--bundle-output') {
      if (i + 1 >= args.length || args[i + 1].startsWith('--')) {
        throw new Error('TSPACK_INSPECT_INVALID_BUNDLE_OPTIONS: --bundle-output requires a value');
      }
      i += 1;
      options.bundleOutput = args[i];
      options.bundle = true;
      continue;
    }
    if (arg === '--selector') { i += 1; options.selector = args[i]; continue; }
    if (arg === '--browser') { i += 1; options.browser = args[i] as typeof options.browser; continue; }
    if (arg === '--host-path') { i += 1; options.hostPath = args[i]; continue; }
    if (arg === '--browser-path') { i += 1; options.browserPath = args[i]; continue; }
    if (arg === '--cdp') { i += 1; options.cdpEndpoint = args[i]; options.browser = 'cdp'; continue; }
    if (arg === '--list-targets') { options.listTargets = true; continue; }
    if (arg === '--target') { i += 1; options.target = args[i]; continue; }
    if (arg === '--target-url') { i += 1; options.targetUrl = args[i]; continue; }
    if (arg === '--viewport') { i += 1; options.viewport = parseViewport(args[i]); continue; }
    if (arg === '--point') { i += 1; options.points.push(parsePoint(args[i])); continue; }
    throw new Error(`unknown flag: ${arg}`);
  }

  return { ...options, url };
}

async function main(): Promise<void> {
  const options = parseInspectCliArgs(process.argv.slice(2));
  await inspectAndWrite(options);
}

main().catch((error: unknown) => {
  let options: InspectOptions | undefined;
  try {
    options = parseInspectCliArgs(process.argv.slice(2));
  } catch {
    // Keep the raw CLI parse failure for non-JSON mode.
  }
  const jsonRequested = options?.json ?? process.argv.slice(2).includes('--json');

  if (jsonRequested) {
    const failure = buildInspectFailureResult(options ?? {
      browser: 'auto',
      viewport: { width: 1440, height: 900 },
      points: [],
      json: true
    }, error);
    process.stdout.write(formatInspectJson(failure));
    process.exit(1);
    return;
  }

  process.stderr.write(`${(error as Error).message}\n`);
  process.exit(1);
});
