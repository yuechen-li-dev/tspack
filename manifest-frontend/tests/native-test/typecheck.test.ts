import fs from "node:fs";
import os from "node:os";
import path from "node:path";
import { describe, expect, it } from "vitest";
import {
  createNativeTestReport,
  formatNativeTestCompactTextReport,
  formatNativeTestJsonReport,
  listNativeTests,
  runNativeTestFiles,
  typecheckNativeTestFile,
  assert,
} from "../../src/native-test";

function makeDir(): string {
  return fs.mkdtempSync(path.join(os.tmpdir(), "tspack-native-typecheck-"));
}

function nativeImportPath(): string {
  return path.resolve(process.cwd(), "src/native-test/index.ts").replace(/\\/g, "/");
}

function writeTestFile(root: string, name: string, source: string): string {
  const filePath = path.join(root, name);
  fs.writeFileSync(filePath, source);
  return filePath;
}

describe("native assert.type typecheck probe", () => {
  it("exposes assert.type at runtime and requires a reason", () => {
    assert.type<string>("ok", "runtime no-op records an assertion reason");
    expect(() => assert.type<string>("ok", "")).toThrow();
  });

  it("passes semantic type assertions in the entry file", () => {
    const root = makeDir();
    const filePath = writeTestFile(
      root,
      "pass.xtest.tsx",
      `
      import { Suite, Fact, assert } from '${nativeImportPath()}';
      function cx(): string { return 'a'; }
      export default (
        <Suite name="types">
          <Fact name="cx returns string">{() => {
            assert.type<string>(cx(), "cx should return a string");
          }}</Fact>
        </Suite>
      );
      `,
    );

    const result = typecheckNativeTestFile(filePath, { rootDir: root });

    expect(result.assertions).toHaveLength(1);
    expect(result.diagnostics).toHaveLength(0);
  });

  it("maps assignability failures to type assertion diagnostics", () => {
    const root = makeDir();
    const filePath = writeTestFile(
      root,
      "fail.xtest.tsx",
      `
      import { Suite, Fact, assert } from '${nativeImportPath()}';
      function cx(): number { return 1; }
      export default (
        <Suite name="types">
          <Fact name="cx returns string">{() => {
            assert.type<string>(cx(), "cx should return a string");
          }}</Fact>
        </Suite>
      );
      `,
    );

    const result = typecheckNativeTestFile(filePath, { rootDir: root });

    expect(result.diagnostics).toHaveLength(1);
    expect(result.diagnostics[0]).toMatchObject({
      code: "TSPACK_TYPE_ASSERTION_FAILED",
      reason: "cx should return a string",
      expectedTypeText: "string",
      localTestId: "types/cx returns string",
      typescriptCode: 2345,
    });
  });

  it("checks imported local source return types", () => {
    const root = makeDir();
    fs.writeFileSync(
      path.join(root, "cx.ts"),
      `export function cx(): number { return 1; }\n`,
    );
    const filePath = writeTestFile(
      root,
      "imported.xtest.tsx",
      `
      import { Suite, Fact, assert } from '${nativeImportPath()}';
      import { cx } from './cx';
      export default (
        <Suite name="types">
          <Fact name="imported cx">{() => {
            assert.type<string>(cx(), "imported cx should return a string");
          }}</Fact>
        </Suite>
      );
      `,
    );

    const result = typecheckNativeTestFile(filePath, { rootDir: root });

    expect(result.diagnostics.map((diagnostic) => diagnostic.code)).toEqual([
      "TSPACK_TYPE_ASSERTION_FAILED",
    ]);
  });

  it("emits reason-required diagnostics for missing or empty reasons", () => {
    const root = makeDir();
    const filePath = writeTestFile(
      root,
      "reason.xtest.tsx",
      `
      import { Suite, Fact, assert } from '${nativeImportPath()}';
      function cx(): string { return 'a'; }
      export default (
        <Suite name="types">
          <Fact name="missing reason">{() => {
            assert.type<string>(cx());
            assert.type<string>(cx(), "");
          }}</Fact>
        </Suite>
      );
      `,
    );

    const result = typecheckNativeTestFile(filePath, { rootDir: root });

    expect(result.diagnostics.map((diagnostic) => diagnostic.code)).toEqual([
      "TSPACK_TYPE_ASSERTION_REASON_REQUIRED",
      "TSPACK_TYPE_ASSERTION_REASON_REQUIRED",
    ]);
  });

  it("integrates type-only assertions into run reports", async () => {
    const root = makeDir();
    writeTestFile(
      root,
      "type-only.xtest.tsx",
      `
      import { Suite, Fact, assert } from '${nativeImportPath()}';
      function cx(): string { return 'a'; }
      export default (
        <Suite name="types">
          <Fact name="type only">{() => {
            assert.type<string>(cx(), "type-only fact is meaningful");
          }}</Fact>
        </Suite>
      );
      `,
    );

    const result = await runNativeTestFiles({ rootDir: root });

    expect(result.results).toHaveLength(1);
    expect(result.results[0].status).toBe("passed");
    expect(result.diagnostics).toHaveLength(0);
  });

  it("surfaces type assertion failures in compact and JSON reports", async () => {
    const root = makeDir();
    writeTestFile(
      root,
      "report.xtest.tsx",
      `
      import { Suite, Fact, assert } from '${nativeImportPath()}';
      function cx(): number { return 1; }
      export default (
        <Suite name="types">
          <Fact name="bad type">{() => {
            assert.type<string>(cx(), "report should show the type failure");
          }}</Fact>
        </Suite>
      );
      `,
    );

    const report = createNativeTestReport(await runNativeTestFiles({ rootDir: root }));
    const compact = formatNativeTestCompactTextReport(report);
    const json = JSON.parse(formatNativeTestJsonReport(report));

    expect(report.tests[0].status).toBe("failed");
    expect(compact).toContain("TSPACK_TYPE_ASSERTION_FAILED");
    expect(json.tests[0].failure.code).toBe("TSPACK_TYPE_ASSERTION_FAILED");
    expect(json.tests[0].failure.details.typescriptCode).toBe("TS2345");
  });

  it("keeps list mode non-executing and non-typechecking", async () => {
    const root = makeDir();
    writeTestFile(
      root,
      "list.xtest.tsx",
      `
      import { Suite, Fact, assert } from '${nativeImportPath()}';
      function cx(): number { return 1; }
      export default (
        <Suite name="types">
          <Fact name="bad but listed">{() => {
            assert.type<string>(cx(), "list should not typecheck this failure");
          }}</Fact>
        </Suite>
      );
      `,
    );

    const listed = await listNativeTests({ rootDir: root });
    const runList = await runNativeTestFiles({ rootDir: root, listOnly: true });

    expect(listed.diagnostics).toHaveLength(0);
    expect(runList.diagnostics).toHaveLength(0);
    expect(runList.results[0].status).toBe("passed");
  });

  it("does not fail a selected run for an unselected bad type assertion", async () => {
    const root = makeDir();
    writeTestFile(
      root,
      "filter.xtest.tsx",
      `
      import { Suite, Fact, assert } from '${nativeImportPath()}';
      function cx(): number { return 1; }
      export default (
        <Suite name="types">
          <Fact name="selected">{() => {
            assert.true(true, "selected runtime assertion passes");
          }}</Fact>
          <Fact name="unselected bad">{() => {
            assert.type<string>(cx(), "unselected type assertion fails");
          }}</Fact>
        </Suite>
      );
      `,
    );

    const result = await runNativeTestFiles({
      rootDir: root,
      filter: "filter.xtest.tsx::types/selected",
    });

    expect(result.results.map((entry) => entry.id)).toEqual([
      "filter.xtest.tsx::types/selected",
    ]);
    expect(result.results[0].status).toBe("passed");
  });
});
