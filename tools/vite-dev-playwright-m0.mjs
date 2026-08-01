import { spawn } from "node:child_process";
import { cp, mkdtemp, readFile, rm, symlink, writeFile } from "node:fs/promises";
import { createRequire } from "node:module";
import { dirname, join, resolve } from "node:path";
import { setTimeout as delay } from "node:timers/promises";

const require = createRequire(import.meta.url);
const { chromium } = require("playwright");
const root = resolve(readOption("--root") ?? "fixtures/tscl-browser-m1");
const tspack = resolve(readOption("--tspack") ?? "tspack");
const frontendPort = Number.parseInt(readOption("--port") ?? "5190", 10);

if (!Number.isInteger(frontendPort) || frontendPort < 1 || frontendPort > 65535) {
  throw new Error("TSPACK_VITE_DEV_TEST_INVALID_PORT: --port must be 1 through 65535.");
}

const fixture = await copyFixture(root);
const frontendURL = `http://127.0.0.1:${frontendPort}`;
const sourcePath = join(fixture, "src", "View.ts");
const originalSource = await readFile(sourcePath, "utf8");
let dev;
let browser;

try {
  dev = spawn(tspack, ["run", "dev", "--root", fixture, "--env", `VITE_PORT=${frontendPort}`, "--ready-timeout", "60"], {
    cwd: dirname(fixture),
    stdio: ["ignore", "pipe", "pipe"],
  });
  const output = captureOutput(dev);
  await waitForHTTP(frontendURL, 60_000);

  browser = await chromium.launch();
  const page = await browser.newPage();
  let loads = 0;
  page.on("load", () => loads++);
  const browserErrors = [];
  page.on("console", message => {
    if (message.type() === "error") browserErrors.push(message.text());
  });
  page.on("pageerror", error => browserErrors.push(error.message));

  await page.goto(frontendURL, { waitUntil: "networkidle" });
  await page.locator("#increment").click();
  await page.waitForFunction(() => document.querySelector("#status")?.textContent?.includes("Count: 1"));
  const api = await page.evaluate(() => fetch("/api/status").then(response => response.json()));
  if (api.status !== "ok") throw new Error(`TSPACK_VITE_DEV_TEST_PROXY_FAILED: ${JSON.stringify(api)}`);

  const marker = "Vite Playwright reload proof";
  await writeFile(sourcePath, originalSource.replace("Browser package call:", `${marker}: Browser package call:`));
  await waitForPageText(page, marker, 30_000, output);
  if (loads < 2) throw new Error("TSPACK_VITE_DEV_TEST_RELOAD_FAILED: successful Copeland rebuild did not cause a browser full reload.");

  await writeFile(sourcePath, originalSource.replace("return `", "const invalid: string = ;\n    return `"));
  await waitForOutput(output, "COPE-PARSE-0002", 30_000);
  const retained = await page.locator("#status").textContent();
  if (!retained?.includes(marker)) throw new Error("TSPACK_VITE_DEV_TEST_LAST_GOOD_PAGE_LOST: failed compile replaced the previous page.");

  await writeFile(sourcePath, originalSource);
  await waitForPageText(page, "Browser package call:", 30_000);
  if (browserErrors.length) throw new Error(`TSPACK_VITE_DEV_TEST_BROWSER_ERRORS: ${browserErrors.join("; ")}`);

  console.log(JSON.stringify({ success: true, frontendURL, loads, backend: api.status }, null, 2));
} finally {
  await browser?.close();
  await stopChild(dev);
	await waitForEndpointClosed(frontendURL, 10_000);
	await waitForEndpointClosed("http://127.0.0.1:5187/api/status", 10_000);
  await removeFixture(fixture);
}

async function copyFixture(source) {
  const destination = await mkdtemp(join(dirname(source), ".tspack-vite-dev-m0-"));
  await cp(source, destination, {
    recursive: true,
    filter: path => !["node_modules", "dist", ".tspack"].includes(path.split(/[\\/]/).at(-1)),
  });
  await symlink(join(source, "node_modules"), join(destination, "node_modules"), "junction");
  return destination;
}

function captureOutput(child) {
  let value = "";
  const append = chunk => { value += chunk.toString(); };
  child.stdout.on("data", append);
  child.stderr.on("data", append);
  return () => value;
}

async function waitForHTTP(url, timeout) {
  const deadline = Date.now() + timeout;
  while (Date.now() < deadline) {
    try {
      const response = await fetch(url);
      if (response.ok) return;
    } catch {
      // Startup is expected to race the readiness probe.
    }
    await delay(150);
  }
  throw new Error(`TSPACK_VITE_DEV_TEST_FRONTEND_TIMEOUT: ${url}`);
}

async function waitForEndpointClosed(url, timeout) {
  const deadline = Date.now() + timeout;
  while (Date.now() < deadline) {
    try {
      await fetch(url);
    } catch {
      return;
    }
    await delay(150);
  }
  throw new Error(`TSPACK_VITE_DEV_TEST_OWNED_CHILD_REMAINS: ${url}`);
}

async function waitForPageText(page, text, timeout, readOutput) {
  const deadline = Date.now() + timeout;
  while (Date.now() < deadline) {
    const value = await page.locator("#status").textContent();
    if (value?.includes(text)) return;
    await delay(150);
  }
  const output = readOutput ? ` Output: ${readOutput()}` : "";
  throw new Error(`TSPACK_VITE_DEV_TEST_PAGE_TIMEOUT: ${text}.${output}`);
}

async function waitForOutput(readOutput, text, timeout) {
  const deadline = Date.now() + timeout;
  while (Date.now() < deadline) {
    if (readOutput().includes(text)) return;
    await delay(150);
  }
  throw new Error(`TSPACK_VITE_DEV_TEST_DIAGNOSTIC_TIMEOUT: ${text}`);
}

async function stopChild(child) {
  if (!child || child.exitCode !== null) return;
  child.kill("SIGTERM");
  const stopped = await Promise.race([
    new Promise(resolve => child.once("exit", resolve)),
    delay(8_000).then(() => false),
  ]);
  if (stopped === false && child.exitCode === null) {
    child.kill("SIGKILL");
    await Promise.race([new Promise(resolve => child.once("exit", resolve)), delay(5_000)]);
  }
}

async function removeFixture(path) {
  let lastError;
  for (let attempt = 0; attempt < 20; attempt++) {
    try {
      await rm(path, { recursive: true, force: true, maxRetries: 2, retryDelay: 100 });
      return;
    } catch (error) {
      lastError = error;
      await delay(250);
    }
  }
  throw lastError;
}

function readOption(name) {
  const index = process.argv.indexOf(name);
  return index >= 0 ? process.argv[index + 1] : undefined;
}
