import fs from "node:fs";
import path from "node:path";
import type { InspectOptions } from "./index.js";
import { INSPECT_ANALYZER_SCRIPT } from "./analyzer.js";
import type {
  InspectBrowserName,
  UIInspectResult,
  CDPTargetSummary,
} from "./types.js";
import { listCdpTargets } from "./cdp.js";
import { launchInspectableHost, validateHostPath } from "./host-launch.js";

export type InspectBackendName =
  | "auto"
  | "vscode"
  | "playwright-chromium"
  | "playwright-webkit"
  | "browser-path"
  | "host-path"
  | "chromium"
  | "webkit"
  | "cdp"
  | "platform-webview";

export type InspectBackendProbe = {
  executablePath?: string;
  reason?: string;
};

type ExistsSync = (filePath: string) => boolean;
type EnvMap = Record<string, string | undefined>;

export type BrowserDiscoveryHost = {
  platform: NodeJS.Platform;
  env: (name: string) => string | undefined;
  exists: ExistsSync;
};

type InspectLaunchFailure = {
  code: string;
  message: string;
  details: string[];
  fixes: string[];
};

const WINDOWS_EDGE_CANDIDATES = [
  ["ProgramFiles(x86)", "Microsoft", "Edge", "Application", "msedge.exe"],
  ["ProgramFiles", "Microsoft", "Edge", "Application", "msedge.exe"],
  ["LocalAppData", "Microsoft", "Edge", "Application", "msedge.exe"],
];

const WINDOWS_CHROME_CANDIDATES = [
  ["ProgramFiles", "Google", "Chrome", "Application", "chrome.exe"],
  ["ProgramFiles(x86)", "Google", "Chrome", "Application", "chrome.exe"],
  ["LocalAppData", "Google", "Chrome", "Application", "chrome.exe"],
];

const WINDOWS_VSCODE_CANDIDATES = [
  ["LocalAppData", "Programs", "Microsoft VS Code", "Code.exe"],
  ["ProgramFiles", "Microsoft VS Code", "Code.exe"],
  ["ProgramFiles(x86)", "Microsoft VS Code", "Code.exe"],
  ["LocalAppData", "Programs", "VSCodium", "VSCodium.exe"],
  ["ProgramFiles", "VSCodium", "VSCodium.exe"],
  ["ProgramFiles(x86)", "VSCodium", "VSCodium.exe"],
];

const CHROMIUM_PATH_NAMES = [
  "chromium",
  "chromium-browser",
  "google-chrome",
  "google-chrome-stable",
  "chrome",
];

const MACOS_CHROMIUM_CANDIDATES = [
  "/Applications/Google Chrome.app/Contents/MacOS/Google Chrome",
  "/Applications/Chromium.app/Contents/MacOS/Chromium",
  "/Applications/Microsoft Edge.app/Contents/MacOS/Microsoft Edge",
];

export function buildInspectAnalyzerExpression(
  selector: string | undefined,
  points: Array<{ x: number; y: number }>,
): string {
  const analyzerArgs = { selector, points };
  return `(${INSPECT_ANALYZER_SCRIPT})(${JSON.stringify(analyzerArgs)})`;
}

function envLookup(env: EnvMap, name: string): string | undefined {
  return env[name];
}

function createBrowserDiscoveryHost(
  env: EnvMap,
  existsSync: ExistsSync,
  platform: NodeJS.Platform = process.platform,
): BrowserDiscoveryHost {
  return {
    platform,
    env: (name: string) => envLookup(env, name),
    exists: existsSync,
  };
}

function joinEnvPath(
  host: BrowserDiscoveryHost,
  segments: string[],
): string | undefined {
  if (segments.length === 0) {
    return undefined;
  }

  const base = host.env(segments[0]);
  if (!base) {
    return undefined;
  }

  if (host.platform === "win32") {
    return path.win32.join(base, ...segments.slice(1));
  }

  return path.posix.join(base, ...segments.slice(1));
}

function findExistingPath(
  candidates: Array<string | undefined>,
  existsSync: ExistsSync = fs.existsSync,
): string | null {
  for (const candidate of candidates) {
    if (!candidate) {
      continue;
    }
    if (existsSync(candidate)) {
      return candidate;
    }
  }
  return null;
}

function windowsExecutableExtensions(host: BrowserDiscoveryHost): string[] {
  const pathext = host.env("PATHEXT") ?? ".COM;.EXE;.BAT;.CMD";
  const extensions = pathext
    .split(";")
    .map((value: string) => value.trim().toLowerCase())
    .filter(Boolean);

  if (extensions.length === 0) {
    return [".exe", ".cmd", ".bat", ".com"];
  }

  return extensions;
}

function findExecutableOnPath(
  names: string[],
  host: BrowserDiscoveryHost = createBrowserDiscoveryHost(
    process.env,
    fs.existsSync,
  ),
): string | null {
  const delimiter = host.platform === "win32" ? ";" : ":";
  const joinPath = host.platform === "win32" ? path.win32.join : path.posix.join;
  const pathEntries = (host.env("PATH") ?? "")
    .split(delimiter)
    .map((value: string) => value.trim())
    .filter(Boolean);
  const extensions =
    host.platform === "win32" ? windowsExecutableExtensions(host) : [""];

  for (const entry of pathEntries) {
    for (const name of names) {
      const hasExtension = path.extname(name) !== "";
      if (hasExtension) {
        const candidate = joinPath(entry, name);
        if (host.exists(candidate)) {
          return candidate;
        }
        continue;
      }

      for (const extension of extensions) {
        const suffix = extension === "" ? "" : extension;
        const candidate = joinPath(entry, `${name}${suffix}`);
        if (host.exists(candidate)) {
          return candidate;
        }
      }
    }
  }

  return null;
}

export function findWindowsChromiumExecutable(
  env: EnvMap = process.env,
  existsSync: ExistsSync = fs.existsSync,
  platform: NodeJS.Platform = "win32",
): string | null {
  const host = createBrowserDiscoveryHost(env, existsSync, platform);
  const explicitCandidates = [
    ...WINDOWS_EDGE_CANDIDATES.map((candidate) => joinEnvPath(host, candidate)),
    ...WINDOWS_CHROME_CANDIDATES.map((candidate) => joinEnvPath(host, candidate)),
  ];

  const explicitMatch = findExistingPath(explicitCandidates, host.exists);
  if (explicitMatch) {
    return explicitMatch;
  }

  return findExecutableOnPath(
    [
      "msedge",
      "msedge.exe",
      "chrome",
      "chrome.exe",
      "chromium",
      "chromium.exe",
    ],
    host,
  );
}

export function findSystemChromiumExecutable(
  env: EnvMap = process.env,
  existsSync: ExistsSync = fs.existsSync,
  platform: NodeJS.Platform = process.platform,
): string | null {
  if (platform === "win32") {
    return findWindowsChromiumExecutable(env, existsSync, platform);
  }

  const host = createBrowserDiscoveryHost(env, existsSync, platform);
  const pathMatch = findExecutableOnPath(CHROMIUM_PATH_NAMES, host);
  if (pathMatch) {
    return pathMatch;
  }

  if (platform === "darwin") {
    return findExistingPath(MACOS_CHROMIUM_CANDIDATES, existsSync);
  }

  return null;
}

type InspectAnalyzerPayload = {
  root: UIInspectResult["root"];
  hitTests: UIInspectResult["hitTests"];
};

function asInspectAnalyzerPayload(value: unknown): InspectAnalyzerPayload {
  if (!value || typeof value !== "object") {
    return { root: null, hitTests: [] };
  }

  const payload = value as Partial<InspectAnalyzerPayload>;
  return {
    root: payload.root ?? null,
    hitTests: Array.isArray(payload.hitTests) ? payload.hitTests : [],
  };
}

type PlatformWebViewProbeResult = {
  os: NodeJS.Platform;
  candidate: "webview2" | "wkwebview" | "webkitgtk";
  checks: string[];
  blocker?: string;
  outcome: "usable" | "not-usable" | "unavailable";
};

function probePlatformWebViewEnvironment(): PlatformWebViewProbeResult {
  const os = process.platform;

  if (os === "win32") {
    const checks = ["platform=windows", "expected-engine=WebView2"];
    return {
      os,
      candidate: "webview2",
      checks,
      blocker:
        "TSPACK_INSPECT_PLATFORM_WEBVIEW_UNAVAILABLE: WebView2 probe is not implemented yet in Node-only backend.",
      outcome: "not-usable",
    };
  }

  if (os === "darwin") {
    const checks = ["platform=macos", "expected-engine=WKWebView"];
    return {
      os,
      candidate: "wkwebview",
      checks,
      blocker:
        "TSPACK_INSPECT_PLATFORM_WEBVIEW_UNAVAILABLE: WKWebView probe is not implemented yet in Node-only backend.",
      outcome: "not-usable",
    };
  }

  const checks: string[] = ["platform=linux", "expected-engine=WebKitGTK"];
  const hasDisplay = Boolean(
    process.env.DISPLAY || process.env.WAYLAND_DISPLAY,
  );
  checks.push(`display=${hasDisplay ? "present" : "missing"}`);

  if (!hasDisplay) {
    return {
      os,
      candidate: "webkitgtk",
      checks,
      blocker:
        "TSPACK_INSPECT_PLATFORM_WEBVIEW_UNAVAILABLE: missing DISPLAY/WAYLAND_DISPLAY for WebKitGTK runtime session.",
      outcome: "unavailable",
    };
  }

  return {
    os,
    candidate: "webkitgtk",
    checks,
    blocker:
      "TSPACK_INSPECT_PLATFORM_WEBVIEW_INIT_FAILED: WebKitGTK backend scaffold exists but runtime host is not wired yet.",
    outcome: "not-usable",
  };
}

export function findVSCodeExecutable(): string | null {
  if (process.platform === "win32") {
    const host = createBrowserDiscoveryHost(process.env, fs.existsSync, "win32");
    const explicitCandidates = WINDOWS_VSCODE_CANDIDATES.map((candidate) =>
      joinEnvPath(host, candidate),
    );
    const explicitMatch = findExistingPath(explicitCandidates, host.exists);
    if (explicitMatch) {
      return explicitMatch;
    }
  }

  return findExecutableOnPath(["code", "code-insiders", "codium", "code-oss"]);
}

export function resolveVSCodeElectronExecutable(wrapperPath: string): string {
  if (process.platform === "win32") {
    return wrapperPath;
  }

  const wrapperName = path.basename(wrapperPath);
  const variants: Record<string, string[]> = {
    code: ["/usr/share/code/code"],
    "code-insiders": ["/usr/share/code-insiders/code-insiders"],
    codium: ["/usr/share/codium/codium"],
    "code-oss": ["/usr/share/code-oss/code-oss"],
    vscodium: ["/usr/share/vscodium/vscodium"],
  };

  const wrapperDirCandidate = path.resolve(
    path.dirname(wrapperPath),
    "..",
    "share",
    wrapperName,
    wrapperName,
  );
  const knownCandidates = variants[wrapperName] ?? [];
  const candidates = [wrapperDirCandidate, ...knownCandidates, wrapperPath];

  for (const candidate of candidates) {
    if (!fs.existsSync(candidate)) {
      continue;
    }
    const stat = fs.statSync(candidate);
    if (stat.isFile()) {
      return candidate;
    }
  }

  return wrapperPath;
}

export async function probeVSCodeElectronBackend(): Promise<InspectBackendProbe> {
  const wrapperPath = findVSCodeExecutable();
  if (!wrapperPath) {
    return { reason: "TSPACK_INSPECT_VSCODE_NOT_FOUND" };
  }
  const executablePath = resolveVSCodeElectronExecutable(wrapperPath);

  try {
    const playwright = await import("playwright");
    const browser = await playwright.chromium.launch({
      executablePath,
      headless: true,
    });
    await browser.close();
    return { executablePath };
  } catch (error: unknown) {
    const message = error instanceof Error ? error.message : String(error);
    return {
      executablePath,
      reason: `TSPACK_INSPECT_VSCODE_ELECTRON_NOT_USABLE: ${message}`,
    };
  }
}

export function isInspectUrlLike(input: string | undefined): boolean {
  return Boolean(input && /^https?:\/\//i.test(input));
}

export function resolveInspectBackend(options: InspectOptions): InspectBackendName {
  if (options.cdpEndpoint) {
    return "cdp";
  }

  if (options.browser !== "auto") {
    return options.browser;
  }

  if (options.hostPath) {
    return "host-path";
  }

  if (options.browserPath) {
    return "browser-path";
  }

  if (isInspectUrlLike(options.url)) {
    return "playwright-chromium";
  }

  return "auto";
}

type PlaywrightBrowserTypeName = "chromium" | "webkit";

type PlaywrightBrowserOverrides = {
  executablePath?: string;
  browserName?: InspectBrowserName;
  backend?: UIInspectResult["browser"]["backend"];
  executableSource?: NonNullable<
    UIInspectResult["browser"]["executable"]
  >["source"];
};

function messageOf(error: unknown): string {
  return error instanceof Error ? error.message : String(error);
}

function boundedMessageOf(error: unknown): string {
  const message = messageOf(error).replace(/\s+/g, " ").trim();
  if (message.length <= 1000) {
    return message;
  }
  return `${message.slice(0, 1000)}…`;
}

function displayMode(): string {
  if (process.env.WAYLAND_DISPLAY) {
    return "wayland";
  }
  if (process.env.DISPLAY) {
    return "x11";
  }
  return "headless/no-display";
}

function isPlaywrightBrowserMissingError(error: unknown): boolean {
  const message = messageOf(error);
  return (
    message.includes("Executable doesn't exist at") ||
    message.includes("Please run the following command to download new browsers") ||
    message.includes("browserType.launch: Executable doesn't exist")
  );
}

function buildBrowserMissingFailure(
  browserTypeName: PlaywrightBrowserTypeName,
  fallbackExecutablePath?: string,
): InspectLaunchFailure {
  const browserLabel = browserTypeName === "chromium" ? "Chromium" : "WebKit";
  const fixes = [
    `Install the Playwright ${browserLabel} browser with: npx playwright install ${browserTypeName}`,
  ];
  const details = [
    `Playwright ${browserLabel} browser runtime is not installed or could not be located.`,
    `Requested launch backend: playwright-${browserTypeName}.`,
    "Attempted executable sources: playwright-managed, system.",
    `Display mode: ${displayMode()}.`,
  ];

  if (fallbackExecutablePath) {
    details.push(`System Chromium fallback found at: ${fallbackExecutablePath}`);
    fixes.push(
      `Pass the browser directly with: --browser-path "${fallbackExecutablePath}"`,
    );
  } else if (browserTypeName === "chromium") {
    details.push(
      "No system Chromium executable was discovered in PATH or standard install locations.",
    );
    fixes.push(
      "Install Chromium, Chrome, or Edge, or pass an explicit browser path with --browser-path.",
    );
  }

  return {
    code: "TSPACK_INSPECT_BROWSER_NOT_FOUND",
    message: `TSPACK_INSPECT_BROWSER_NOT_FOUND: Playwright ${browserLabel} browser is unavailable`,
    details,
    fixes,
  };
}

function buildBrowserLaunchFailure(error: unknown): InspectLaunchFailure {
  const underlying = boundedMessageOf(error);
  return {
    code: "TSPACK_INSPECT_BROWSER_LAUNCH_FAILED",
    message: `TSPACK_INSPECT_BROWSER_LAUNCH_FAILED: ${underlying}`,
    details: [
      "Requested launch backend: playwright.",
      `Display mode: ${displayMode()}.`,
      underlying,
    ],
    fixes: [],
  };
}

function throwLaunchFailure(failure: InspectLaunchFailure): never {
  const extraDetails = [...failure.details, ...failure.fixes];
  if (extraDetails.length === 0) {
    throw new Error(failure.message);
  }
  throw new Error(`${failure.message} | ${extraDetails.join(" | ")}`);
}

async function inspectWithPlaywrightBrowser(
  options: InspectOptions,
  browserTypeName: PlaywrightBrowserTypeName,
  overrides?: PlaywrightBrowserOverrides,
): Promise<UIInspectResult> {
  let playwright: typeof import("playwright");
  try {
    playwright = await import("playwright");
  } catch {
    throw new Error("TSPACK_INSPECT_BROWSER_LAUNCH_FAILED");
  }

  const browserType = playwright[browserTypeName];
  let browser: Awaited<ReturnType<typeof browserType.launch>> | undefined;
  let selectedOverrides = overrides;
  try {
    browser = await browserType.launch({
      executablePath: selectedOverrides?.executablePath,
    });
  } catch (error: unknown) {
    if (
      browserTypeName === "chromium" &&
      !selectedOverrides?.executablePath &&
      isPlaywrightBrowserMissingError(error)
    ) {
      const fallbackExecutablePath = findSystemChromiumExecutable();
      if (fallbackExecutablePath) {
        try {
          browser = await browserType.launch({
            executablePath: fallbackExecutablePath,
          });
          selectedOverrides = {
            ...selectedOverrides,
            executablePath: fallbackExecutablePath,
            executableSource: "system",
          };
        } catch (fallbackError: unknown) {
          if (isPlaywrightBrowserMissingError(fallbackError)) {
            throwLaunchFailure(
              buildBrowserMissingFailure(
                browserTypeName,
                fallbackExecutablePath,
              ),
            );
          }
          throwLaunchFailure(buildBrowserLaunchFailure(fallbackError));
        }
      } else {
        throwLaunchFailure(buildBrowserMissingFailure(browserTypeName));
      }
    }

    if (!browser) {
      if (error instanceof Error && error.message.startsWith("TSPACK_INSPECT_")) {
        throw error;
      }
      if (isPlaywrightBrowserMissingError(error)) {
        throwLaunchFailure(buildBrowserMissingFailure(browserTypeName));
      }
      throwLaunchFailure(buildBrowserLaunchFailure(error));
    }
  }

  if (!browser) {
    throw new Error("TSPACK_INSPECT_BROWSER_LAUNCH_FAILED");
  }

  try {
    const page = await browser.newPage({
      viewport: {
        width: options.viewport.width,
        height: options.viewport.height,
      },
    });
    await page.goto(options.url as string, { waitUntil: "load" });

    const expression = buildInspectAnalyzerExpression(
      options.selector,
      options.points,
    );
    const payload = asInspectAnalyzerPayload(await page.evaluate(expression));
    if (options.selector && !payload.root) {
      throw new Error("TSPACK_INSPECT_SELECTOR_NOT_FOUND");
    }

    return {
      target: { url: options.url as string },
      browser: {
        name: selectedOverrides?.browserName ?? browserTypeName,
        backend: selectedOverrides?.backend ?? "playwright",
        launchBackend:
          browserTypeName === "chromium"
            ? "playwright-chromium"
            : "playwright-webkit",
        version: browser.version(),
        executable: {
          source:
            selectedOverrides?.executableSource ??
            (selectedOverrides?.executablePath ? "explicit" : "playwright-managed"),
          path: selectedOverrides?.executablePath,
        },
      },
      viewport: {
        width: options.viewport.width,
        height: options.viewport.height,
      },
      root: payload.root,
      hitTests: payload.hitTests,
      diagnostics: [],
    };
  } catch (error: unknown) {
    if (error instanceof Error && error.message.startsWith("TSPACK_INSPECT_")) {
      throw error;
    }
    throw new Error("TSPACK_INSPECT_PAGE_LOAD_FAILED");
  } finally {
    await browser.close();
  }
}

async function inspectWithChromium(
  options: InspectOptions,
  overrides?: PlaywrightBrowserOverrides,
): Promise<UIInspectResult> {
  return inspectWithPlaywrightBrowser(options, "chromium", overrides);
}

async function inspectWithWebKit(
  options: InspectOptions,
): Promise<UIInspectResult> {
  return inspectWithPlaywrightBrowser(options, "webkit", {
    browserName: "webkit",
    backend: "playwright",
  });
}

function selectCdpTarget(
  targets: CDPTargetSummary[],
  targetInput?: string,
  targetUrlSubstring?: string,
): CDPTargetSummary {
  if (targetInput) {
    const asIndex = Number(targetInput);
    if (Number.isInteger(asIndex)) {
      const target = targets.find((item) => item.index === asIndex);
      if (!target) throw new Error("TSPACK_INSPECT_CDP_TARGET_NOT_FOUND");
      return target;
    }
    const byId = targets.find((item) => item.id === targetInput);
    if (!byId) throw new Error("TSPACK_INSPECT_CDP_TARGET_NOT_FOUND");
    return byId;
  }

  if (targetUrlSubstring) {
    const matches = targets.filter((item) =>
      item.url.includes(targetUrlSubstring),
    );
    if (matches.length === 0)
      throw new Error("TSPACK_INSPECT_CDP_TARGET_NOT_FOUND");
    if (matches.length > 1)
      throw new Error("TSPACK_INSPECT_CDP_TARGET_AMBIGUOUS");
    return matches[0];
  }

  if (targets.length === 0)
    throw new Error("TSPACK_INSPECT_CDP_TARGET_NOT_FOUND");
  return targets[0];
}

async function inspectWithCdp(
  options: InspectOptions,
): Promise<UIInspectResult> {
  let playwright: typeof import("playwright");
  try {
    playwright = await import("playwright");
  } catch {
    throw new Error("TSPACK_INSPECT_CDP_CONNECT_FAILED");
  }

  let browser:
    | Awaited<ReturnType<typeof playwright.chromium.connectOverCDP>>
    | undefined;
  try {
    const list = await listCdpTargets(options.cdpEndpoint as string);
    const target = selectCdpTarget(
      list.targets,
      options.target,
      options.targetUrl,
    );
    if (!target.webSocketDebuggerUrl) {
      throw new Error("TSPACK_INSPECT_CDP_TARGET_UNSUPPORTED");
    }

    browser = await playwright.chromium.connectOverCDP(
      options.cdpEndpoint as string,
    );
    const contexts = browser.contexts();
    const pages = contexts.flatMap((context) => context.pages());
    let page = pages.find((candidate) => candidate.url() === target.url);
    if (!page) {
      page = pages[0];
    }
    if (!page) {
      throw new Error("TSPACK_INSPECT_CDP_TARGET_NOT_FOUND");
    }

    if (options.url) {
      await page.goto(options.url, { waitUntil: "load" });
    }

    if (options.viewport.width > 0 && options.viewport.height > 0) {
      await page.setViewportSize({
        width: options.viewport.width,
        height: options.viewport.height,
      });
    }

    const expression = buildInspectAnalyzerExpression(
      options.selector,
      options.points,
    );
    const payload = asInspectAnalyzerPayload(await page.evaluate(expression));
    if (options.selector && !payload.root) {
      throw new Error("TSPACK_INSPECT_SELECTOR_NOT_FOUND");
    }

    return {
      target: { url: options.url ?? page.url() },
      browser: {
        name: "cdp",
        backend: "cdp",
        launchBackend: "cdp",
        executable: { source: "connected" },
      },
      viewport: {
        width: options.viewport.width,
        height: options.viewport.height,
      },
      root: payload.root,
      hitTests: payload.hitTests,
      diagnostics: [],
    };
  } catch (error: unknown) {
    if (error instanceof Error && error.message.startsWith("TSPACK_INSPECT_")) {
      throw error;
    }
    throw new Error("TSPACK_INSPECT_CDP_EVALUATION_FAILED");
  } finally {
    await browser?.close();
  }
}
export async function runInspect(
  options: InspectOptions,
): Promise<UIInspectResult> {
  if (options.url && !isInspectUrlLike(options.url)) {
    throw new Error("TSPACK_INSPECT_INVALID_TARGET");
  }

  const hostPath = options.hostPath;
  const browserPath = options.browserPath;
  const configuredBackend = options.browser;
  const backend = resolveInspectBackend(options);

  if (hostPath && browserPath) {
    throw new Error("TSPACK_INSPECT_INVALID_BACKEND_OPTIONS");
  }

  if (options.cdpEndpoint && (hostPath || browserPath)) {
    throw new Error("TSPACK_INSPECT_INVALID_BACKEND_OPTIONS");
  }

  if (
    options.cdpEndpoint &&
    configuredBackend !== "auto" &&
    configuredBackend !== "cdp"
  ) {
    throw new Error("TSPACK_INSPECT_INVALID_BACKEND_OPTIONS");
  }

  if (
    (hostPath || browserPath) &&
    configuredBackend !== "auto" &&
    configuredBackend !== "host-path" &&
    configuredBackend !== "browser-path"
  ) {
    throw new Error("TSPACK_INSPECT_INVALID_BACKEND_OPTIONS");
  }

  if (backend === "platform-webview") {
    const probe = probePlatformWebViewEnvironment();
    const checks = probe.checks.join(",");
    const blocker =
      probe.blocker ?? "TSPACK_INSPECT_PLATFORM_WEBVIEW_UNAVAILABLE";
    throw new Error(`${blocker} [${checks}]`);
  }

  if (backend === "cdp") {
    return inspectWithCdp(options);
  }

  if (backend === "playwright-chromium" || backend === "chromium") {
    return inspectWithChromium(options, {
      browserName: "chromium",
      backend: "playwright",
    });
  }

  if (backend === "playwright-webkit" || backend === "webkit") {
    return inspectWithWebKit(options);
  }

  if (backend === "browser-path") {
    if (!browserPath || !fs.existsSync(browserPath)) {
      throw new Error("TSPACK_INSPECT_BROWSER_PATH_NOT_FOUND");
    }
    return inspectWithChromium(options, {
      executablePath: browserPath,
      browserName: "chromium",
      backend: "browser-path",
      executableSource: "explicit",
    });
  }

  if (backend === "host-path" || hostPath || browserPath) {
    const explicitHostPath = hostPath ?? browserPath;
    if (!explicitHostPath) {
      throw new Error("TSPACK_INSPECT_HOST_PATH_NOT_FOUND");
    }
    validateHostPath(explicitHostPath);
    const launchedHost = await launchInspectableHost({
      executablePath: explicitHostPath,
    });
    try {
      return await inspectWithCdp({
        ...options,
        cdpEndpoint: launchedHost.endpoint,
        browser: "cdp",
      }).then((result) => ({
        ...result,
        browser: {
          ...result.browser,
          name: "chromium",
          backend: "host-path",
          launchBackend: "host-path",
          executable: {
            source: "explicit",
            path: explicitHostPath,
          },
        },
      }));
    } catch (error: unknown) {
      if (
        error instanceof Error &&
        error.message.startsWith("TSPACK_INSPECT_")
      ) {
        throw error;
      }
      throw new Error("TSPACK_INSPECT_HOST_CDP_ENDPOINT_FAILED");
    } finally {
      await launchedHost.cleanup();
    }
  }

  if (backend === "vscode") {
    const probe = await probeVSCodeElectronBackend();
    if (!probe.executablePath) {
      throw new Error(probe.reason ?? "TSPACK_INSPECT_VSCODE_NOT_FOUND");
    }
    if (probe.reason) {
      throw new Error(probe.reason);
    }
    return inspectWithChromium(options, {
      executablePath: probe.executablePath,
      browserName: "chromium",
      backend: "vscode",
      executableSource: "explicit",
    });
  }

  if (backend === "auto") {
    return inspectWithChromium(options, {
      browserName: "chromium",
      backend: "playwright",
    });
  }

  throw new Error("TSPACK_INSPECT_BROWSER_UNSUPPORTED");
}
