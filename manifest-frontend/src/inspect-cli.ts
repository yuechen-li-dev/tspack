import { inspectAndWrite, parsePoint, parseViewport } from './inspect/index.js';

async function main(): Promise<void> {
  const args = process.argv.slice(2);
  const cursor = args[0] === 'inspect' ? 1 : 0;
  let url: string | undefined;
  const options = {
    browser: 'auto' as const,
    viewport: { width: 1440, height: 900 },
    selector: undefined as string | undefined,
    points: [] as Array<{ x: number; y: number }>,
    json: false,
    out: undefined as string | undefined,
    text: undefined as string | undefined,
    browserPath: undefined as string | undefined,
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
    if (arg === '--selector') { i += 1; options.selector = args[i]; continue; }
    if (arg === '--browser') { i += 1; options.browser = args[i] as typeof options.browser; continue; }
    if (arg === '--browser-path') { i += 1; options.browserPath = args[i]; continue; }
    if (arg === '--cdp') { i += 1; options.cdpEndpoint = args[i]; options.browser = 'cdp'; continue; }
    if (arg === '--list-targets') { options.listTargets = true; continue; }
    if (arg === '--target') { i += 1; options.target = args[i]; continue; }
    if (arg === '--target-url') { i += 1; options.targetUrl = args[i]; continue; }
    if (arg === '--viewport') { i += 1; options.viewport = parseViewport(args[i]); continue; }
    if (arg === '--point') { i += 1; options.points.push(parsePoint(args[i])); continue; }
    throw new Error(`unknown flag: ${arg}`);
  }

  await inspectAndWrite({ ...options, url });
}

main().catch((error: unknown) => {
  process.stderr.write(`${(error as Error).message}\n`);
  process.exit(1);
});
