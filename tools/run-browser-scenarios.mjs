import { createRequire } from "node:module";
import { mkdir, readFile, writeFile } from "node:fs/promises";
import { dirname, resolve } from "node:path";

const require = createRequire(import.meta.url);
const { chromium } = require("playwright");
const baseUrl = readOption("--url");
const scenarioFile = readOption("--scenario");

if (!baseUrl || !scenarioFile) {
  fail("TSPACK_SCENARIO_INVALID_ARGS", "Usage: node run-browser-scenarios.mjs --url <url> --scenario <scenario.json>.");
}

const scenarioPath = resolve(scenarioFile);
const declaration = JSON.parse(await readFile(scenarioPath, "utf8"));
validateDeclaration(declaration);
const artifactDirectory = resolve(dirname(scenarioPath), declaration.artifactDirectory);
await mkdir(artifactDirectory, { recursive: true });

const report = { command: "scenario", baseUrl, scenarios: [], diagnostics: { console: [], page: [], request: [] } };
let browser;

try {
  browser = await chromium.launch();
  for (const item of declaration.scenarios) {
    const context = await browser.newContext({ viewport: item.viewport, reducedMotion: item.reducedMotion ?? "no-preference" });
    await context.grantPermissions(["clipboard-read", "clipboard-write"], { origin: new URL(baseUrl).origin });
    const page = await context.newPage();
    const localDiagnostics = { console: [], page: [], request: [] };
    page.on("console", message => {
      if (message.type() === "error") localDiagnostics.console.push(message.text());
    });
    page.on("pageerror", error => localDiagnostics.page.push(error.message));
    page.on("requestfailed", request => localDiagnostics.request.push({ url: request.url(), error: request.failure()?.errorText ?? "unknown" }));

    await page.goto(new URL(item.path ?? "/", baseUrl).toString(), { waitUntil: "networkidle" });
    await runChecks(page, item.assertions ?? []);
    for (const step of item.steps ?? []) await runStep(page, step);
    await runChecks(page, item.after ?? []);

    let screenshot = null;
    if (item.screenshot) {
      screenshot = resolve(artifactDirectory, item.screenshot);
      await mkdir(dirname(screenshot), { recursive: true });
      await page.screenshot({ path: screenshot, fullPage: item.fullPage ?? false });
    }
    if (localDiagnostics.console.length || localDiagnostics.page.length || localDiagnostics.request.length) {
      fail("TSPACK_SCENARIO_BROWSER_DIAGNOSTICS", `${item.name} observed browser diagnostics.`);
    }
    report.scenarios.push({ name: item.name, viewport: item.viewport, screenshot, diagnostics: localDiagnostics });
    await context.close();
  }

  const reportPath = resolve(artifactDirectory, "scenario-report.json");
  await writeFile(reportPath, `${JSON.stringify(report, null, 2)}\n`);
  process.stdout.write(`${JSON.stringify({ success: true, report: reportPath, scenarios: report.scenarios }, null, 2)}\n`);
} catch (error) {
  const message = error instanceof Error ? error.message : String(error);
  process.stderr.write(`${message}\n`);
  process.exitCode = 1;
} finally {
  await browser?.close();
}

async function runStep(page, step) {
  if (step.kind === "click") return page.locator(step.selector).click();
  if (step.kind === "press") return page.keyboard.press(step.key);
  if (step.kind === "check") return runChecks(page, [step.check]);
  fail("TSPACK_SCENARIO_STEP_INVALID", `Unknown step kind ${JSON.stringify(step.kind)}.`);
}

async function runChecks(page, checks) {
  for (const check of checks) {
    if (check.kind === "visible") await page.locator(check.selector).waitFor({ state: "visible" });
    else if (check.kind === "hidden") await page.locator(check.selector).waitFor({ state: "hidden" });
    else if (check.kind === "count") {
      const count = await page.locator(check.selector).count();
      if (count !== check.value) fail("TSPACK_SCENARIO_ASSERTION_FAILED", `${check.selector} count was ${count}, expected ${check.value}.`);
    } else if (check.kind === "text") {
      await page.locator(check.selector).filter({ hasText: check.value }).waitFor({ state: "visible" });
    } else if (check.kind === "no-horizontal-overflow") {
      const overflow = await page.evaluate(() => document.documentElement.scrollWidth > window.innerWidth);
      if (overflow) fail("TSPACK_SCENARIO_ASSERTION_FAILED", "Page has horizontal overflow.");
    } else if (check.kind === "class") {
      const classes = await page.locator(check.selector).getAttribute("class");
      if (!classes?.split(/\s+/).includes(check.value)) fail("TSPACK_SCENARIO_ASSERTION_FAILED", `${check.selector} did not have class ${check.value}.`);
    } else if (check.kind === "focused") {
      const focused = await page.locator(check.selector).evaluate(element => document.activeElement === element);
      if (!focused) fail("TSPACK_SCENARIO_ASSERTION_FAILED", `${check.selector} was not focused.`);
    } else if (check.kind === "reduced-motion") {
      const reduced = await page.evaluate(() => window.matchMedia("(prefers-reduced-motion: reduce)").matches);
      if (!reduced) fail("TSPACK_SCENARIO_ASSERTION_FAILED", "Reduced-motion media preference was not applied.");
    } else fail("TSPACK_SCENARIO_ASSERTION_INVALID", `Unknown assertion kind ${JSON.stringify(check.kind)}.`);
  }
}

function validateDeclaration(value) {
  if (!value || typeof value !== "object" || typeof value.artifactDirectory !== "string" || !Array.isArray(value.scenarios)) {
    fail("TSPACK_SCENARIO_DECLARATION_INVALID", "Expected artifactDirectory and scenarios[].");
  }
  for (const item of value.scenarios) {
    if (!item.name || !item.viewport || !Number.isInteger(item.viewport.width) || !Number.isInteger(item.viewport.height)) {
      fail("TSPACK_SCENARIO_DECLARATION_INVALID", "Every scenario requires name and integer viewport width/height.");
    }
  }
}

function readOption(name) {
  const index = process.argv.indexOf(name);
  return index >= 0 ? process.argv[index + 1] : undefined;
}

function fail(code, message) {
  throw new Error(`${code}: ${message}`);
}
