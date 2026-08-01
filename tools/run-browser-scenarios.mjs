import { createRequire } from "node:module";
import { mkdir, readFile, writeFile } from "node:fs/promises";
import { dirname, resolve } from "node:path";
import { fail, runChecks, validateDeclaration } from "./browser-scenario-assertions.mjs";

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
    page.on("response", response => {
      if (response.status() >= 400) localDiagnostics.request.push({ url: response.url(), status: response.status() });
    });

    let screenshot = null;
    try {
      await page.goto(new URL(item.path ?? "/", baseUrl).toString(), { waitUntil: "networkidle" });
      const assertions = await runChecks(page, item.assertions ?? []);
      for (const step of item.steps ?? []) await runStep(page, step);
      const after = await runChecks(page, item.after ?? []);

      if (item.screenshot) {
        screenshot = resolve(artifactDirectory, item.screenshot);
        await mkdir(dirname(screenshot), { recursive: true });
        await page.screenshot({ path: screenshot, fullPage: item.fullPage ?? false });
      }
      if (localDiagnostics.console.length || localDiagnostics.page.length || localDiagnostics.request.length) {
        fail("TSPACK_SCENARIO_BROWSER_DIAGNOSTICS", `${item.name} observed browser diagnostics.`);
      }
      report.scenarios.push({ name: item.name, viewport: item.viewport, screenshot, assertions: [...assertions, ...after], diagnostics: localDiagnostics });
    } catch (error) {
      const failureScreenshot = resolve(artifactDirectory, `${safeArtifactName(item.name)}-failure.png`);
      await page.screenshot({ path: failureScreenshot, fullPage: item.fullPage ?? false }).catch(() => {});
      const message = error instanceof Error ? error.message : String(error);
      const diagnosticSummary = formatDiagnostics(localDiagnostics);
      fail("TSPACK_SCENARIO_ASSERTION_FAILED", `${item.name}: ${message}${diagnosticSummary} Screenshot: ${failureScreenshot}`);
    } finally {
      await context.close();
    }
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

function safeArtifactName(name) {
  return String(name).replace(/[^A-Za-z0-9._-]+/g, "-");
}

function formatDiagnostics(diagnostics) {
  const values = [
    ...diagnostics.console.map(message => `console=${message}`),
    ...diagnostics.page.map(message => `page=${message}`),
    ...diagnostics.request.map(item => `request=${item.url}${item.status ? ` status=${item.status}` : ` error=${item.error}`}`),
  ];
  return values.length === 0 ? "" : ` Diagnostics: ${values.join("; ")}.`;
}

function readOption(name) {
  const index = process.argv.indexOf(name);
  return index >= 0 ? process.argv[index + 1] : undefined;
}
