import { createRequire } from "node:module";
import { spawn } from "node:child_process";

const require = createRequire(import.meta.url);
const { chromium } = require("playwright");

const url = readOption("--url");
const root = readOption("--root");
const tspack = readOption("--tspack");
if (!url) {
  fail("TSPACK_COPELAND_REACT_URL_REQUIRED", "Usage: node tools/Prove-CopelandReactPlaywright.mjs --url <http-url> [--tspack <path> --root <project-root>].");
}
if (Boolean(tspack) !== Boolean(root)) {
  fail("TSPACK_COPELAND_REACT_LIFECYCLE_REQUIRED", "Provide both --tspack and --root to have TSPack own host lifecycle.");
}

const consoleFailures = [];
const requestFailures = [];
const pageFailures = [];
let browser;
let lifecycle;

try {
  if (tspack && root) {
    lifecycle = startLifecycle(tspack, root);
    await waitForReady(url, lifecycle);
  }

  browser = await chromium.launch();
  const page = await browser.newPage();
  page.on("console", (message) => {
    if (message.type() === "error") {
      consoleFailures.push(message.text());
    }
  });
  page.on("pageerror", (error) => pageFailures.push(error.message));
  page.on("requestfailed", (request) => requestFailures.push({
    url: request.url(),
    failure: request.failure()?.errorText ?? "unknown request failure"
  }));

  await page.goto(url, { waitUntil: "networkidle" });
  await page.getByRole("heading", { name: "Hello Copeland" }).waitFor();
  await page.getByRole("button", { name: "Call CLR operation" }).click();
  const expectedGreeting = "Hello, React. This message was compiled from Copeland.";
  await page.locator("p").filter({ hasText: expectedGreeting }).waitFor();
  const greeting = await page.locator("p").textContent();
  if (greeting !== expectedGreeting) {
    fail("TSPACK_COPELAND_REACT_ASSERTION_FAILED", `Expected CLR greeting ${JSON.stringify(expectedGreeting)}; received ${JSON.stringify(greeting)}.`);
  }

  if (consoleFailures.length || requestFailures.length || pageFailures.length) {
    fail("TSPACK_COPELAND_REACT_BROWSER_FAILURE", "Browser console, request, or page failures were observed.");
  }

  process.stdout.write(`${JSON.stringify({
    success: true,
    url,
    interaction: "Call CLR operation",
    greeting,
    consoleFailures,
    requestFailures,
    pageFailures,
    lifecycle: lifecycle ? lifecycle.evidence() : undefined
  }, null, 2)}\n`);
} catch (error) {
  const message = error instanceof Error ? error.message : String(error);
  process.stderr.write(`${message}\n`);
  process.exitCode = 1;
} finally {
  await browser?.close();
  await lifecycle?.stop();
}

function readOption(name) {
  const index = process.argv.indexOf(name);
  if (index < 0 || index + 1 >= process.argv.length) {
    return undefined;
  }

  return process.argv[index + 1];
}

function fail(code, message) {
  const error = new Error(`${code}: ${message}`);
  error.code = code;
  throw error;
}

function startLifecycle(executable, projectRoot) {
  const stdout = [];
  const stderr = [];
  const process = spawn(executable, ["run", "web", "--root", projectRoot], {
    cwd: projectRoot,
    windowsHide: true
  });
  process.stdout.on("data", (data) => stdout.push(data.toString()));
  process.stderr.on("data", (data) => stderr.push(data.toString()));

  return {
    evidence() {
      return { stdout: stdout.join(""), stderr: stderr.join("") };
    },
    async stop() {
      if (process.exitCode !== null || process.killed) {
        return;
      }

      process.kill();
      await new Promise((resolve) => process.once("exit", resolve));
    }
  };
}

async function waitForReady(target, lifecycle) {
  const deadline = Date.now() + 30_000;
  while (Date.now() < deadline) {
    try {
      const response = await fetch(target);
      if (response.ok) {
        return;
      }
    } catch {
      // The TSPack process remains the lifecycle owner while readiness retries.
    }

    await new Promise((resolve) => setTimeout(resolve, 200));
  }

  fail("TSPACK_COPELAND_REACT_READY_TIMEOUT", `TSPack did not make ${target} ready. ${lifecycle.evidence().stderr}`);
}
