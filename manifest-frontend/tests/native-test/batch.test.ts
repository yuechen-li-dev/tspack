import fs from "node:fs";
import os from "node:os";
import path from "node:path";
import { describe, expect, it } from "vitest";
import {
  chooseBatchWorkerCount,
  createNativeTestReport,
  formatNativeTestCompactTextReport,
  formatNativeTestJsonReport,
  listNativeTests,
  runNativeTestFiles,
} from "../../src/native-test";

function makeDir(): string {
  return fs.mkdtempSync(path.join(os.tmpdir(), "tspack-native-batch-"));
}

function importPath(): string {
  return path
    .resolve(process.cwd(), "src/native-test/index.ts")
    .replace(/\\/g, "/");
}

function writeTestFile(root: string, name: string, body: string): void {
  fs.writeFileSync(path.join(root, name), body);
}

describe("native xTest batch execution", () => {
  it("runs multiple files concurrently and reports in discovery order", async () => {
    const root = makeDir();
    const marker = path.join(root, "markers.log").replace(/\\/g, "/");
    writeTestFile(
      root,
      "a-slow.xtest.tsx",
      `
      import fs from 'node:fs';
      import { Suite, Fact, assert } from '${importPath()}';
      const sleep = (ms) => new Promise((resolve) => setTimeout(resolve, ms));
      export default (
        <Suite name="a">
          <Fact name="slow">{async () => {
            await sleep(80);
            fs.appendFileSync('${marker}', 'A\\n');
            assert.true(true, 'slow passed');
          }}</Fact>
        </Suite>
      );
    `,
    );
    writeTestFile(
      root,
      "b-fast.xtest.tsx",
      `
      import fs from 'node:fs';
      import { Suite, Fact, assert } from '${importPath()}';
      export default (
        <Suite name="b">
          <Fact name="fast">{() => {
            fs.appendFileSync('${marker}', 'B\\n');
            assert.true(true, 'fast passed');
          }}</Fact>
        </Suite>
      );
    `,
    );

    const oldWorkers = process.env.TSPACK_TEST_BATCH_WORKERS;
    process.env.TSPACK_TEST_BATCH_WORKERS = "2";
    try {
      const result = await runNativeTestFiles({ rootDir: root, batch: true });
      const report = createNativeTestReport(result);

      expect(report.summary.passed).toBe(2);
      expect(report.tests.map((test) => test.id)).toEqual([
        "a-slow.xtest.tsx::a/slow",
        "b-fast.xtest.tsx::b/fast",
      ]);
      expect(fs.readFileSync(marker, "utf8").trim().split("\n")).toEqual([
        "B",
        "A",
      ]);
    } finally {
      if (oldWorkers === undefined) {
        delete process.env.TSPACK_TEST_BATCH_WORKERS;
      } else {
        process.env.TSPACK_TEST_BATCH_WORKERS = oldWorkers;
      }
    }
  });

  it("aggregates failures without fail-fast and keeps within-file order sequential", async () => {
    const root = makeDir();
    const order = path.join(root, "order.log").replace(/\\/g, "/");
    writeTestFile(
      root,
      "a-order.xtest.tsx",
      `
      import fs from 'node:fs';
      import { Suite, Fact, assert } from '${importPath()}';
      export default (
        <Suite name="order">
          <Fact name="one">{() => { fs.appendFileSync('${order}', '1'); assert.true(true, 'one'); }}</Fact>
          <Fact name="two">{() => { fs.appendFileSync('${order}', '2'); assert.true(true, 'two'); }}</Fact>
        </Suite>
      );
    `,
    );
    writeTestFile(
      root,
      "b-fail.xtest.tsx",
      `
      import { Suite, Fact, assert } from '${importPath()}';
      export default (<Suite name="fail"><Fact name="bad">{() => { assert.equal(1, 2, 'fails'); }}</Fact></Suite>);
    `,
    );

    const result = await runNativeTestFiles({ rootDir: root, batch: true });
    const report = createNativeTestReport(result);

    expect(fs.readFileSync(order, "utf8")).toBe("12");
    expect(report.summary.passed).toBe(2);
    expect(report.summary.failed).toBe(1);
    expect(report.tests.map((test) => test.id)).toEqual([
      "a-order.xtest.tsx::order/one",
      "a-order.xtest.tsx::order/two",
      "b-fail.xtest.tsx::fail/bad",
    ]);
  });

  it("filters before scheduling and composes with compact output", async () => {
    const root = makeDir();
    const skippedMarker = path.join(root, "skipped.txt").replace(/\\/g, "/");
    writeTestFile(
      root,
      "a-skip-import.xtest.tsx",
      `
      import fs from 'node:fs';
      import { Suite, Fact, assert } from '${importPath()}';
      fs.writeFileSync('${skippedMarker}', 'imported');
      export default (<Suite name="skipme"><Fact name="body">{() => { assert.true(true, 'not selected'); }}</Fact></Suite>);
    `,
    );
    writeTestFile(
      root,
      "b-selected.xtest.tsx",
      `
      import { Suite, Fact, assert } from '${importPath()}';
      export default (<Suite name="selected"><Fact name="bad">{() => { assert.equal('x', 'y', 'selected failure'); }}</Fact></Suite>);
    `,
    );

    const result = await runNativeTestFiles({
      rootDir: root,
      filter: "selected",
      batch: true,
    });
    const report = createNativeTestReport(result);
    const compactText = formatNativeTestCompactTextReport(report);

    expect(fs.existsSync(skippedMarker)).toBe(false);
    expect(report.tests.map((test) => test.id)).toEqual([
      "b-selected.xtest.tsx::selected/bad",
    ]);
    expect(compactText).toContain("FAIL b-selected.xtest.tsx::selected/bad");
    expect(compactText).not.toContain("PASS");
  });

  it("updates snapshots per file and preserves deterministic JSON order", async () => {
    const root = makeDir();
    writeTestFile(
      root,
      "a-snapshot.xtest.tsx",
      `
      import { Suite, Fact, expect } from '${importPath()}';
      export default (<Suite name="snapA"><Fact name="write">{() => { expect.snapshotText('A\\n', 'value').because('write A'); }}</Fact></Suite>);
    `,
    );
    writeTestFile(
      root,
      "b-snapshot.xtest.tsx",
      `
      import { Suite, Fact, expect } from '${importPath()}';
      export default (<Suite name="snapB"><Fact name="write">{() => { expect.snapshotJson({ b: 2 }, 'value').because('write B'); }}</Fact></Suite>);
    `,
    );

    const result = await runNativeTestFiles({
      rootDir: root,
      batch: true,
      updateSnapshots: true,
    });
    const report = createNativeTestReport(result);
    const parsed = JSON.parse(formatNativeTestJsonReport(report));

    expect(report.summary.snapshotsUpdated).toBe(2);
    expect(parsed.tests.map((test: { id: string }) => test.id)).toEqual([
      "a-snapshot.xtest.tsx::snapA/write",
      "b-snapshot.xtest.tsx::snapB/write",
    ]);
    expect(
      fs.existsSync(
        path.join(
          root,
          "__snapshots__",
          "a-snapshot.xtest.tsx",
          "value.snap.txt",
        ),
      ),
    ).toBe(true);
    expect(
      fs.existsSync(
        path.join(
          root,
          "__snapshots__",
          "b-snapshot.xtest.tsx",
          "value.snap.json",
        ),
      ),
    ).toBe(true);
  });

  it("keeps project fixtures isolated across files", async () => {
    const root = makeDir();
    const fixture = path.join(root, "fixture");
    fs.mkdirSync(fixture);
    fs.writeFileSync(path.join(fixture, "base.txt"), "base");
    const fixturePath = fixture.replace(/\\/g, "/");

    for (const fileName of ["a-project.xtest.tsx", "b-project.xtest.tsx"]) {
      writeTestFile(
        root,
        fileName,
        `
        import fs from 'node:fs';
        import { Suite, Fact, Project, assert } from '${importPath()}';
        export default (
          <Suite name="${fileName}">
            <Fact name="sandbox">
              <Project from="${fixturePath}" />
              {async ({ project }) => {
                await project.writeText('marker.txt', '${fileName}', 'marker');
                assert.equal(await project.readText('marker.txt'), '${fileName}', 'own marker');
                assert.true(!fs.existsSync('${fixturePath}/marker.txt'), 'fixture not mutated');
              }}
            </Fact>
          </Suite>
        );
      `,
      );
    }

    const result = await runNativeTestFiles({ rootDir: root, batch: true });
    const report = createNativeTestReport(result);

    expect(report.summary.failed).toBe(0);
    expect(report.summary.passed).toBe(2);
    expect(fs.existsSync(path.join(fixture, "marker.txt"))).toBe(false);
  });

  it("ignores batch for list mode and exposes bounded worker count policy", async () => {
    const root = makeDir();
    writeTestFile(
      root,
      "listed.xtest.tsx",
      `
      import fs from 'node:fs';
      import { Suite, Fact, assert } from '${importPath()}';
      fs.writeFileSync('${path.join(root, "executed.txt").replace(/\\/g, "/")}', 'imported');
      export default (<Suite name="listed"><Fact name="one">{() => { assert.true(true, 'one'); }}</Fact></Suite>);
    `,
    );

    const listed = await listNativeTests({ rootDir: root });

    expect(listed.tests.map((test) => test.id)).toEqual([
      "listed.xtest.tsx::listed/one",
    ]);
    expect(fs.existsSync(path.join(root, "executed.txt"))).toBe(false);
    expect(chooseBatchWorkerCount(0)).toBe(0);
    expect(chooseBatchWorkerCount(1)).toBe(1);
    expect(chooseBatchWorkerCount(50)).toBeLessThanOrEqual(8);
  });
});
