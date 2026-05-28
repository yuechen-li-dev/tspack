import fs from "node:fs";
import os from "node:os";
import path from "node:path";
import { describe, expect, it } from "vitest";
import {
  createNativeTestReport,
  formatNativeTestJsonReport,
  formatNativeTestTextReport,
  formatNativeTestCompactTextReport,
  listNativeTests,
  nativeTestExitCode,
  runNativeTestFiles,
} from "../../src/native-test";

function makeDir(): string {
  return fs.mkdtempSync(path.join(os.tmpdir(), "tspack-native-report-"));
}

describe("native test listing/filter/report", () => {
  it("lists facts and theory cases without execution", async () => {
    const root = path.resolve(process.cwd(), "tests/native-test/fixtures");
    const listed = await listNativeTests({ rootDir: root });
    expect(
      listed.tests.some((test) =>
        test.id.includes("side-effect.xtest.tsx::side/body"),
      ),
    ).toBe(true);
    expect(listed.tests.some((test) => test.kind === "theory-case")).toBe(true);
  });

  it("lists callback-before-cases theories without executing bodies and keeps filters stable", async () => {
    const root = makeDir();
    const importPath = path
      .resolve(process.cwd(), "src/native-test/index.ts")
      .replace(/\\/g, "/");
    const marker = path.join(root, "listed-marker.txt").replace(/\\/g, "/");
    fs.writeFileSync(
      path.join(root, "before.xtest.tsx"),
      `
      import fs from 'node:fs';
      import { Suite, Theory, Case, assert } from '${importPath}';
      export default (
        <Suite name="before">
          <Theory name="letters">
            {({ value }) => {
              fs.writeFileSync('${marker}', String(value));
              assert.true(value === 'A' || value === 'B', 'letter case asserts');
            }}
            <Case value="A" />
            <Case value="B" />
          </Theory>
        </Suite>
      );
    `,
    );

    const listed = await listNativeTests({ rootDir: root });
    expect(listed.diagnostics).toHaveLength(0);
    expect(listed.tests.map((test) => test.id)).toEqual([
      "before.xtest.tsx::before/letters[0]",
      "before.xtest.tsx::before/letters[1]",
    ]);
    expect(fs.existsSync(marker)).toBe(false);

    const filteredBySuffix = await runNativeTestFiles({
      rootDir: root,
      filter: "[1]",
    });
    expect(filteredBySuffix.results.map((test) => test.id)).toEqual([
      "before.xtest.tsx::before/letters[1]",
    ]);

    const copiedId = listed.tests[0].id;
    const filteredByCopiedId = await runNativeTestFiles({
      rootDir: root,
      filter: copiedId,
    });
    expect(filteredByCopiedId.results.map((test) => test.id)).toEqual([
      copiedId,
    ]);
  });

  it("reports invalid theory structure diagnostics without vacuous passing entries", async () => {
    const root = makeDir();
    const importPath = path
      .resolve(process.cwd(), "src/native-test/index.ts")
      .replace(/\\/g, "/");
    fs.writeFileSync(
      path.join(root, "invalid.xtest.tsx"),
      `
      import { Suite, Theory, Case, assert } from '${importPath}';
      export default (
        <Suite name="invalid">
          <Theory name="zero">{() => { assert.true(true, 'should not pass'); }}</Theory>
          <Theory name="missing"><Case value={1} /></Theory>
          <Theory name="duplicate"><Case value={1} />{() => { assert.true(true, 'first'); }}{() => { assert.true(true, 'second'); }}</Theory>
        </Suite>
      );
    `,
    );

    const listed = await listNativeTests({ rootDir: root });
    const result = await runNativeTestFiles({ rootDir: root });
    const report = createNativeTestReport(result);
    const diagnosticCodes = result.diagnostics.map(
      (diagnostic) => diagnostic.code,
    );
    const failureCodes = result.results.map(
      (test) => (test.error as { code?: string } | undefined)?.code,
    );

    expect(listed.tests).toHaveLength(0);
    expect(result.results.every((test) => test.status === "failed")).toBe(true);
    expect(failureCodes).toEqual([
      "TSPACK_TEST_THEORY_NO_CASES",
      "TSPACK_TEST_THEORY_MISSING_BODY",
      "TSPACK_TEST_THEORY_DUPLICATE_BODY",
    ]);
    expect(diagnosticCodes).toContain("TSPACK_TEST_THEORY_NO_CASES");
    expect(diagnosticCodes).toContain("TSPACK_TEST_THEORY_MISSING_BODY");
    expect(diagnosticCodes).toContain("TSPACK_TEST_THEORY_DUPLICATE_BODY");
    expect(nativeTestExitCode(report)).toBe(1);
  });

  it("uses the same root-relative IDs for list, run, and filters", async () => {
    const root = makeDir();
    const importPath = path
      .resolve(process.cwd(), "src/native-test/index.ts")
      .replace(/\\/g, "/");
    const nested = path.join(root, "src");
    fs.mkdirSync(nested, { recursive: true });
    fs.writeFileSync(
      path.join(nested, "ids.xtest.tsx"),
      `
      import { Suite, Fact, Theory, Case, assert } from '${importPath}';
      export default (
        <Suite name="ids">
          <Fact name="copy me">{() => { assert.true(true, 'copy'); }}</Fact>
          <Theory name="case filter">
            <Case value={0} />
            <Case value={1} />
            <Case value={2} />
            {() => { assert.true(true, 'case'); }}
          </Theory>
        </Suite>
      );
    `,
    );

    const listed = await listNativeTests({ rootDir: root });
    const listedIds = listed.tests.map((test) => test.id).sort();
    expect(listedIds).toContain("src/ids.xtest.tsx::ids/copy me");
    expect(listedIds).toContain("src/ids.xtest.tsx::ids/case filter[2]");
    expect(listedIds.every((id) => !path.isAbsolute(id.split("::")[0]))).toBe(
      true,
    );

    const run = await runNativeTestFiles({ rootDir: root });
    const runIds = run.results.map((test) => test.id).sort();
    expect(runIds).toEqual(listedIds);

    const copiedId = "src/ids.xtest.tsx::ids/copy me";
    const copiedFilter = await runNativeTestFiles({
      rootDir: root,
      filter: copiedId,
    });
    expect(copiedFilter.results.map((test) => test.id)).toEqual([copiedId]);

    const caseFilter = await runNativeTestFiles({
      rootDir: root,
      filter: "[2]",
    });
    expect(caseFilter.results.map((test) => test.id)).toEqual([
      "src/ids.xtest.tsx::ids/case filter[2]",
    ]);
  });

  it("filter no match is diagnostic and exit code 1", async () => {
    const root = path.resolve(process.cwd(), "tests/native-test/fixtures");
    const result = await runNativeTestFiles({
      rootDir: root,
      filter: "no-such-test",
    });
    const report = createNativeTestReport(result);
    expect(result.results).toHaveLength(0);
    expect(
      report.diagnostics.some(
        (entry) => entry.code === "TSPACK_TEST_FILTER_NO_MATCH",
      ),
    ).toBe(true);
    expect(nativeTestExitCode(report)).toBe(1);
  });

  it("filters before import and provides deterministic reports", async () => {
    const root = makeDir();
    const sideEffectFile = path.join(root, "side-effect.xtest.tsx");
    const targetFile = path.join(root, "target.xtest.tsx");

    fs.writeFileSync(
      sideEffectFile,
      `
      import fs from 'node:fs';
      import path from 'node:path';
      import { Suite, Fact } from '${path.resolve(process.cwd(), "src/native-test/index.ts").replace(/\\/g, "/")}';
      fs.writeFileSync(path.join('${root.replace(/\\/g, "/")}', 'executed.txt'), 'ran');
      export default (<Suite name="side"><Fact name="boom">{() => {}}</Fact></Suite>);
    `,
    );

    fs.writeFileSync(
      targetFile,
      `
      import { Suite, Fact, Theory, Case, assert, skip, Artifact } from '${path.resolve(process.cwd(), "src/native-test/index.ts").replace(/\\/g, "/")}';
      export default (
        <Suite name="report">
          <Fact name="pass">
            <Artifact name="out" path="out.json" format="json" />
            {async ({ artifact }) => { await artifact.writeJson('out', { ok: true }, 'proof'); assert.true(true, 'pass'); }}
          </Fact>
          <Fact name="skip">{() => { skip('not now'); }}</Fact>
          <Fact name="near fail">{() => { assert.near(3.2, 3.14159, 0.001, 'circle'); }}</Fact>
          <Theory name="many"><Case input={1} /><Case input={2} />{() => { assert.true(true, 'ok'); }}</Theory>
        </Suite>
      );
    `,
    );

    const result = await runNativeTestFiles({
      rootDir: root,
      filter: "report",
    });
    expect(fs.existsSync(path.join(root, "executed.txt"))).toBe(false);

    const report = createNativeTestReport(result);
    const text = formatNativeTestTextReport(report);
    const compactText = formatNativeTestCompactTextReport(report);
    const json1 = formatNativeTestJsonReport(report);
    const json2 = formatNativeTestJsonReport(report);

    expect(text.includes("PASS target.xtest.tsx::report/pass")).toBe(true);
    expect(text.includes("SKIP target.xtest.tsx::report/skip")).toBe(true);
    expect(text.includes("FAIL target.xtest.tsx::report/near fail")).toBe(true);
    expect(text.includes("tolerance")).toBe(true);
    expect(text.includes("difference")).toBe(true);
    expect(text.includes("Artifacts:")).toBe(true);
    expect(compactText.includes("PASS target.xtest.tsx::report/pass")).toBe(false);
    expect(compactText.includes("SKIP target.xtest.tsx::report/skip")).toBe(true);
    expect(compactText.includes("FAIL target.xtest.tsx::report/near fail")).toBe(true);
    expect(compactText.includes("Summary:")).toBe(true);
    expect(JSON.parse(json1).summary.total).toBe(5);
    expect(json1).toBe(json2);
    expect(nativeTestExitCode(report)).toBe(1);
  });
});

describe("native test compact reporting", () => {
  it("hides passed tests and keeps an all-pass report summary-only", () => {
    const report = createNativeTestReport({
      results: [
        { id: "alpha.xtest.tsx::suite/one", name: "one", status: "passed" },
        { id: "alpha.xtest.tsx::suite/two", name: "two", status: "passed" },
      ],
      diagnostics: [],
    });

    const text = formatNativeTestCompactTextReport(report);

    expect(text).toContain("Summary:");
    expect(text).toContain("  passed: 2");
    expect(text).toContain("  failed: 0");
    expect(text).toContain("  skipped: 0");
    expect(text).not.toContain("PASS");
    expect(text).not.toContain("alpha.xtest.tsx::suite/one");
    expect(text).not.toContain("alpha.xtest.tsx::suite/two");
    expect(text).not.toContain(".");
    expect(text).not.toContain("\n\n");
  });

  it("shows failed tests with full assertion details", () => {
    const report = createNativeTestReport({
      results: [
        { id: "alpha.xtest.tsx::suite/pass", name: "pass", status: "passed" },
        {
          id: "alpha.xtest.tsx::suite/fail",
          name: "fail",
          status: "failed",
          error: Object.assign(new Error("equal failed"), {
            code: "TSPACK_ASSERT_EQUAL_FAILED",
            reason: "values should match",
            assertion: "equal",
            expected: "only",
            actual: "a",
            tolerance: 0.5,
          }),
        },
      ],
      diagnostics: [],
    });

    const text = formatNativeTestCompactTextReport(report);

    expect(text).toContain("FAIL alpha.xtest.tsx::suite/fail");
    expect(text).toContain("code: TSPACK_ASSERT_EQUAL_FAILED");
    expect(text).toContain("message: equal failed");
    expect(text).toContain("reason: values should match");
    expect(text).toContain('expected: "only"');
    expect(text).toContain('actual: "a"');
    expect(text).toContain("assertion: equal");
    expect(text).toContain("tolerance: 0.5");
    expect(text).not.toContain("alpha.xtest.tsx::suite/pass");
    expect(text).not.toContain("PASS");
  });

  it("shows skipped tests with reasons", () => {
    const report = createNativeTestReport({
      results: [
        { id: "env.xtest.tsx::env/pass", name: "pass", status: "passed" },
        {
          id: "env.xtest.tsx::env/browser only",
          name: "browser only",
          status: "skipped",
          skipReason: "requires browser runtime",
        },
      ],
      diagnostics: [],
    });

    const text = formatNativeTestCompactTextReport(report);

    expect(text).toContain("SKIP env.xtest.tsx::env/browser only");
    expect(text).toContain("reason: requires browser runtime");
    expect(text).not.toContain("env.xtest.tsx::env/pass");
  });

  it("preserves discovery diagnostics and leaves JSON formatting unaffected", () => {
    const report = createNativeTestReport({
      results: [],
      diagnostics: [
        {
          code: "TSPACK_TEST_THEORY_NO_CASES",
          message: "Theory requires at least one Case child",
          file: "invalid.xtest.tsx",
          severity: "error",
        },
      ],
    });

    const text = formatNativeTestCompactTextReport(report);
    const json = formatNativeTestJsonReport(report);

    expect(text).toContain("Diagnostics:");
    expect(text).toContain("TSPACK_TEST_THEORY_NO_CASES error");
    expect(text).toContain("Theory requires at least one Case child");
    expect(text).toContain("file: invalid.xtest.tsx");
    expect(JSON.parse(json).diagnostics[0].code).toBe(
      "TSPACK_TEST_THEORY_NO_CASES",
    );
  });
});
