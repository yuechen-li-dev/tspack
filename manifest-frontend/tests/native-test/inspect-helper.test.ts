import http from "node:http";
import fs from "node:fs";
import os from "node:os";
import path from "node:path";
import { describe, expect, it } from "vitest";
import {
  Fact,
  Suite,
  assert,
  createInspectHelper,
  createNativeTestReport,
  formatNativeTestCompactTextReport,
  formatNativeTestJsonReport,
  inspect,
  runSuite,
  typecheckNativeTestFile,
  type InspectResult,
} from "../../src/native-test";
import type { InspectOptions as BackendInspectOptions } from "../../src/inspect/index";

async function chromiumAvailability(): Promise<{
  available: boolean;
  reason?: string;
}> {
  try {
    const playwright = await import("playwright");
    const browser = await playwright.chromium.launch();
    await browser.close();
    return { available: true };
  } catch (error: unknown) {
    const message = error instanceof Error ? error.message : String(error);
    return { available: false, reason: `PLAYWRIGHT_UNAVAILABLE: ${message}` };
  }
}

const chromium = await chromiumAvailability();
const itWithChromium = chromium.available ? it : it.skip;

if (!chromium.available) {
  console.warn(chromium.reason);
}

function sampleResult(): InspectResult {
  return {
    target: { url: "http://127.0.0.1:5173" },
    browser: { name: "chromium", backend: "playwright" },
    viewport: { width: 1280, height: 800 },
    root: {
      id: "node-1",
      tag: "main",
      role: "main",
      name: "Home",
      text: "Home Save",
      bounds: { x: 0, y: 0, width: 1280, height: 800 },
      visible: true,
      children: [
        {
          id: "node-2",
          tag: "button",
          role: "button",
          name: "Save",
          text: "Save",
          bounds: { x: 20, y: 20, width: 80, height: 32 },
          visible: true,
          focusable: true,
          source: {
            raw: "src/components/Button.tsx:42:7",
            file: "src/components/Button.tsx",
            line: 42,
            column: 7,
            component: "Button",
            symbol: "Button.Primary",
          },
          children: [],
        },
        {
          id: "node-3",
          tag: "p",
          role: "status",
          name: "Draft saved",
          text: "Draft saved",
          bounds: { x: 20, y: 80, width: 120, height: 20 },
          visible: false,
          children: [],
        },
      ],
    },
    hitTests: [
      {
        point: { x: 25, y: 25 },
        elements: [
          {
            id: "node-2",
            tag: "button",
            role: "button",
            name: "Save",
            text: "Save",
            bounds: { x: 20, y: 20, width: 80, height: 32 },
            visible: true,
            focusable: true,
            source: {
              raw: "src/components/Button.tsx:42:7",
              file: "src/components/Button.tsx",
              line: 42,
              column: 7,
              component: "Button",
              symbol: "Button.Primary",
            },
            children: [],
          },
        ],
      },
    ],
    diagnostics: [],
  };
}

describe("native inspect helper", () => {
  it("maps inspect.url options to the shared inspect backend", async () => {
    const calls: BackendInspectOptions[] = [];
    const helper = createInspectHelper(async (options) => {
      calls.push(options);
      return sampleResult();
    });

    await helper.url("http://127.0.0.1:5173", {
      browser: "playwright-chromium",
      selector: "#root",
      viewport: "1280x800",
      points: [{ x: 100, y: 200 }],
    });

    expect(calls).toEqual([
      {
        url: "http://127.0.0.1:5173",
        browser: "playwright-chromium",
        viewport: { width: 1280, height: 800 },
        selector: "#root",
        points: [{ x: 100, y: 200 }],
        json: true,
      },
    ]);
  });

  it("resolves inspect.runTarget from the CLI-owned ready target URL", async () => {
    const calls: BackendInspectOptions[] = [];
    const helper = createInspectHelper(async (options) => {
      calls.push(options);
      return sampleResult();
    });
    const previousUrl = process.env.TSPACK_TEST_RUN_TARGET_URL;
    process.env.TSPACK_TEST_RUN_TARGET_URL = "http://127.0.0.1:5198";
    try {
      await helper.runTarget({ selector: "[role=alert]" });
    } finally {
      if (previousUrl === undefined) {
        delete process.env.TSPACK_TEST_RUN_TARGET_URL;
      } else {
        process.env.TSPACK_TEST_RUN_TARGET_URL = previousUrl;
      }
    }

    expect(calls).toHaveLength(1);
    expect(calls[0]).toMatchObject({
      url: "http://127.0.0.1:5198",
      selector: "[role=alert]",
      browser: "chromium",
    });
  });

  it("fails clearly when inspect.runTarget has no CLI-owned target", async () => {
    const helper = createInspectHelper(async () => sampleResult());
    const previousUrl = process.env.TSPACK_TEST_RUN_TARGET_URL;
    delete process.env.TSPACK_TEST_RUN_TARGET_URL;
    try {
      await expect(helper.runTarget()).rejects.toMatchObject({
        code: "TSPACK_TEST_RUN_TARGET_URL_MISSING",
      });
    } finally {
      if (previousUrl !== undefined) {
        process.env.TSPACK_TEST_RUN_TARGET_URL = previousUrl;
      }
    }
  });

  it("maps inspect.cdp target options to the shared inspect backend", async () => {
    const calls: BackendInspectOptions[] = [];
    const helper = createInspectHelper(async (options) => {
      calls.push(options);
      return sampleResult();
    });

    await helper.cdp("http://127.0.0.1:9229", {
      target: 0,
      selector: ".statusbar",
      viewport: { width: 900, height: 600 },
      points: [{ x: 1, y: 2 }],
    });

    expect(calls).toEqual([
      {
        browser: "cdp",
        viewport: { width: 900, height: 600 },
        selector: ".statusbar",
        points: [{ x: 1, y: 2 }],
        json: true,
        cdpEndpoint: "http://127.0.0.1:9229",
        target: "0",
        targetUrl: undefined,
      },
    ]);
  });

  it("exposes pure traversal helpers on the inspect namespace", () => {
    const ui = sampleResult();

    expect(inspect.flatten(ui.root).map((node) => node.id)).toEqual([
      "node-1",
      "node-2",
      "node-3",
    ]);
    expect(inspect.findByRole(ui.root, "button", "Save")?.id).toBe("node-2");
    expect(inspect.findByText(ui.root, /Save/)?.id).toBe("node-1");
  });

  it("builds deterministic UI context bundles for native JSON snapshots", async () => {
    const ui = sampleResult();
    const save = inspect.findByRole(ui.root, "button", "Save");
    const first = await inspect.bundle(ui, save, {
      selectionReason: "save button regression context",
    });
    const second = await inspect.bundle(ui, save, {
      selectionReason: "save button regression context",
    });

    expect(first).toEqual(second);
    expect(first).toMatchObject({
      version: 1,
      kind: "tspack.uiContext",
      selection: { nodeId: "node-2", path: [0] },
      node: { role: "button", name: "Save" },
    });
  });

  it("assert.inspect helpers pass for existing inspect JSON facts", () => {
    const ui = sampleResult();
    const save = inspect.findByRole(ui.root, "button", "Save");
    const hiddenStatus = inspect.findByRole(ui.root, "status", "Draft saved");

    assert.inspect.exists(save, "save button should exist");
    assert.inspect.visible(save, "save button should be visible");
    assert.inspect.hidden(hiddenStatus, "draft status starts hidden");
    assert.inspect.role(save, "button", "save control should keep button role");
    assert.inspect.name(
      save,
      "Save",
      "save control should keep accessible name",
    );
    assert.inspect.focusable(
      save,
      true,
      "save control should remain keyboard focusable",
    );
    assert.inspect.boundsWithin(
      save,
      {
        minWidth: 80,
        minHeight: 32,
        maxWidth: 100,
        maxHeight: 40,
        minX: 0,
        minY: 0,
        maxX: 100,
        maxY: 100,
      },
      "save button should have a usable click target",
    );
    assert.inspect.hitIncludes(
      ui.hitTests[0],
      { role: "button", name: "Save", tag: "button" },
      "hit test should include the save button",
    );
    assert.inspect.source(
      save,
      {
        file: "src/components/Button.tsx",
        component: "Button",
        symbol: "Button.Primary",
      },
      "save button should retain source hints",
    );
  });

  it("assert.inspect helpers fail cleanly with compact diagnostics", () => {
    const ui = sampleResult();
    const save = inspect.findByRole(ui.root, "button", "Save");
    const failures: Array<{
      call: () => void;
      code: string;
      reason: string;
      detailKey?: string;
    }> = [
      {
        call: () => assert.inspect.exists(null, "missing node should fail"),
        code: "TSPACK_ASSERT_INSPECT_EXISTS_FAILED",
        reason: "missing node should fail",
      },
      {
        call: () => assert.inspect.visible(null, "null cannot be visible"),
        code: "TSPACK_ASSERT_INSPECT_VISIBLE_FAILED",
        reason: "null cannot be visible",
      },
      {
        call: () => assert.inspect.hidden(save, "visible node is not hidden"),
        code: "TSPACK_ASSERT_INSPECT_HIDDEN_FAILED",
        reason: "visible node is not hidden",
      },
      {
        call: () =>
          assert.inspect.role(save, "link", "role mismatch should fail"),
        code: "TSPACK_ASSERT_INSPECT_ROLE_FAILED",
        reason: "role mismatch should fail",
      },
      {
        call: () =>
          assert.inspect.name(save, "Submit", "name mismatch should fail"),
        code: "TSPACK_ASSERT_INSPECT_NAME_FAILED",
        reason: "name mismatch should fail",
      },
      {
        call: () =>
          assert.inspect.focusable(
            save,
            false,
            "focusability mismatch should fail",
          ),
        code: "TSPACK_ASSERT_INSPECT_FOCUSABLE_FAILED",
        reason: "focusability mismatch should fail",
      },
      {
        call: () =>
          assert.inspect.boundsWithin(
            save,
            { minWidth: 120 },
            "small bounds should fail",
          ),
        code: "TSPACK_ASSERT_INSPECT_BOUNDS_FAILED",
        reason: "small bounds should fail",
        detailKey: "failedConstraints",
      },
      {
        call: () =>
          assert.inspect.hitIncludes(
            ui.hitTests[0],
            { role: "link" },
            "missing hit role should fail",
          ),
        code: "TSPACK_ASSERT_INSPECT_HIT_FAILED",
        reason: "missing hit role should fail",
        detailKey: "elements",
      },
      {
        call: () =>
          assert.inspect.source(
            save,
            { file: "src/components/Other.tsx" },
            "source mismatch should fail",
          ),
        code: "TSPACK_ASSERT_INSPECT_SOURCE_FAILED",
        reason: "source mismatch should fail",
        detailKey: "failedFields",
      },
    ];

    for (const expected of failures) {
      let thrown: unknown;
      try {
        expected.call();
      } catch (error) {
        thrown = error;
      }

      const failure = thrown as {
        code?: string;
        reason?: string;
        assertion?: string;
        details?: Record<string, unknown>;
      };
      expect(failure.code).toBe(expected.code);
      expect(failure.reason).toBe(expected.reason);
      expect(failure.assertion).toMatch(/^inspect\./);
      expect(failure.details?.assertionKind).toBe(failure.assertion);
      if (expected.detailKey) {
        expect(failure.details?.[expected.detailKey]).toBeDefined();
      }
    }
  });

  it("reports inspect assertion details in compact and JSON reports", () => {
    let thrown: unknown;
    try {
      assert.inspect.boundsWithin(
        inspect.findByRole(sampleResult().root, "button", "Save"),
        { minWidth: 120 },
        "save button should meet target size",
      );
    } catch (error) {
      thrown = error;
    }

    const report = createNativeTestReport({
      results: [
        {
          id: "inspect.xtest.tsx::inspect/bounds",
          name: "bounds",
          status: "failed",
          error: thrown as Error,
        },
      ],
      diagnostics: [],
    });

    const compact = formatNativeTestCompactTextReport(report);
    expect(compact).toContain("TSPACK_ASSERT_INSPECT_BOUNDS_FAILED");
    expect(compact).toContain("save button should meet target size");
    expect(compact).toContain("failedConstraints");

    const json = JSON.parse(formatNativeTestJsonReport(report));
    expect(json.tests[0].failure.code).toBe(
      "TSPACK_ASSERT_INSPECT_BOUNDS_FAILED",
    );
    expect(json.tests[0].failure.details.failedConstraints).toEqual([
      "width 80 < minWidth 120",
    ]);
  });

  it("does not count inspect observation as a native test assertion", async () => {
    const helper = createInspectHelper(async () => sampleResult());
    const root = Suite(
      { name: "inspect" },
      Fact({ name: "only observes" }, async () => {
        await helper.url("http://127.0.0.1:5173");
      }),
    );

    const results = await runSuite(root, {});

    expect(results[0].status).toBe("failed");
    expect((results[0].error as { code?: string }).code).toBe(
      "TSPACK_TEST_NO_ASSERTION",
    );
  });

  it("allows inspect data to be asserted after observation", async () => {
    const helper = createInspectHelper(async () => sampleResult());
    const root = Suite(
      { name: "inspect" },
      Fact({ name: "asserts" }, async () => {
        const ui = await helper.url("http://127.0.0.1:5173");
        assert.equal(ui.root?.role, "main", "main landmark should be present");
      }),
    );

    const results = await runSuite(root, {});

    expect(results[0].status).toBe("passed");
  });

  it("counts inspect assertions as native test assertion activity", async () => {
    const helper = createInspectHelper(async () => sampleResult());
    const root = Suite(
      { name: "inspect" },
      Fact({ name: "inspect assertion only" }, async () => {
        const ui = await helper.url("http://127.0.0.1:5173");
        assert.inspect.visible(ui.root, "main landmark should be visible");
      }),
    );

    const results = await runSuite(root, {});

    expect(results[0].status).toBe("passed");
  });

  it("preserves inspect diagnostic codes on backend failures", async () => {
    const helper = createInspectHelper(async () => {
      throw new Error("TSPACK_INSPECT_SELECTOR_NOT_FOUND");
    });

    await expect(
      helper.url("http://127.0.0.1:5173", { selector: "#missing" }),
    ).rejects.toMatchObject({
      code: "TSPACK_INSPECT_SELECTOR_NOT_FOUND",
      message: "TSPACK_INSPECT_SELECTOR_NOT_FOUND",
    });
  });

  it("exposes inspect.url and inspect.cdp in the native type surface", () => {
    const root = fs.mkdtempSync(
      path.join(os.tmpdir(), "tspack-inspect-types-"),
    );
    const filePath = path.join(root, "inspect.xtest.tsx");
    fs.writeFileSync(
      filePath,
      `
      export default (
        <Suite name="inspect types">
          <Fact name="url options">{async () => {
            const ui = await inspect.url("http://127.0.0.1:5173", {
              browser: "chromium",
              selector: "main",
              viewport: "1280x800",
              points: [{ x: 1, y: 2 }],
            });
            const runTargetUi = await inspect.runTarget({ selector: "main" });
            const bundle = await inspect.bundle(ui, ui.root ?? undefined, { selectionReason: "typed bundle" });
            const cdp = await inspect.cdp("http://127.0.0.1:9229", {
              target: 0,
              selector: ".statusbar",
              points: [{ x: 3, y: 4 }],
            });
            assert.inspect.exists(ui.root, "inspect assertion exists helper is typed");
            assert.inspect.visible(ui.root, "inspect assertion visible helper is typed");
            assert.inspect.hidden(ui.root, "inspect assertion hidden helper is typed");
            assert.inspect.role(ui.root, "main", "inspect assertion role helper is typed");
            assert.inspect.name(ui.root, "Home", "inspect assertion name helper is typed");
            assert.inspect.focusable(ui.root, false, "inspect assertion focusable helper is typed");
            assert.inspect.boundsWithin(ui.root, { minWidth: 1, minHeight: 1, maxX: 10000, maxY: 10000 }, "inspect assertion bounds helper is typed");
            assert.inspect.hitIncludes(ui.hitTests[0], { role: "button", name: "Save", tag: "button" }, "inspect assertion hit helper is typed");
            assert.inspect.source(ui.root, { file: "src/pages/Home.tsx", component: "Home", symbol: "Home" }, "inspect assertion source helper is typed");
            assert.type<string | undefined>(ui.root?.role, "url inspect root role is typed");
            assert.type<string | undefined>(ui.root?.source?.file, "url inspect source file is typed");
            assert.type<number | undefined>(ui.root?.source?.line, "url inspect source line is typed");
            assert.type<string>(cdp.target.url, "cdp inspect target url is typed");
            assert.type<string>(runTargetUi.target.url, "run target inspect URL is typed");
            assert.type<1>(bundle.version, "UI context bundle version is typed");
          }}</Fact>
        </Suite>
      );
      `,
    );

    const result = typecheckNativeTestFile(filePath, { rootDir: root });

    expect(result.diagnostics).toEqual([]);
  });

  itWithChromium(
    "can inspect a tiny local page when Playwright Chromium is available",
    async () => {
      const server = http.createServer((request, response) => {
        response.setHeader("content-type", "text/html");
        response.end(`<!doctype html>
        <main role="main" data-tspack-source="src/pages/Home.tsx">
          <button
            data-tspack-source="src/components/Button.tsx:42:7"
            data-tspack-component="Button"
            data-tspack-symbol="Button.Primary"
          >Save</button>
          <div
            role="alert"
            aria-label="Save failed"
            data-tspack-source="src/components/Toast.tsx:15:5"
            data-tspack-component="Toast"
            data-tspack-symbol="Toast.Error"
          >
            <span>Unable to save</span>
            <button
              disabled
              data-tspack-source="src/components/Toast.tsx:25:7"
              data-tspack-component="Toast"
              data-tspack-symbol="Toast.DismissButton"
            >Dismiss</button>
          </div>
          <span data-tspack-source="src/components/Badge.tsx:17">Ready</span>
          <div data-tspack-source="src/components/Broken.tsx:line">Broken</div>
          <p>No source</p>
        </main>`);
      });

      await new Promise<void>((resolve) => {
        server.listen(0, "127.0.0.1", () => resolve());
      });

      try {
        const address = server.address();
        if (!address || typeof address === "string") {
          throw new Error("failed to bind local server");
        }

        const ui = await inspect.url(`http://127.0.0.1:${address.port}`, {
          browser: "chromium",
          selector: "main",
          viewport: "800x600",
          points: [{ x: 10, y: 10 }],
        });

        expect(ui.root?.role).toBe("main");
        expect(ui.root?.visible).toBe(true);
        expect(ui.root?.source).toMatchObject({
          raw: "src/pages/Home.tsx",
          file: "src/pages/Home.tsx",
        });

        const button = ui.root?.children.find((node) => node.role === "button");
        expect(button?.source).toMatchObject({
          raw: "src/components/Button.tsx:42:7",
          file: "src/components/Button.tsx",
          line: 42,
          column: 7,
          component: "Button",
          symbol: "Button.Primary",
        });

        const alert = inspect.findByRole(ui.root, "alert", "Save failed");
        const dismiss = inspect.findByRole(alert, "button", "Dismiss");
        assert.inspect.exists(alert, "live alert should be discoverable by role");
        assert.inspect.visible(alert, "live alert should be visible");
        assert.inspect.role(alert, "alert", "live toast should retain alert role");
        assert.inspect.name(
          alert,
          "Save failed",
          "live toast should retain its accessible name",
        );
        assert.inspect.focusable(
          dismiss,
          false,
          "disabled dismiss button should not be focusable",
        );
        assert.inspect.source(
          dismiss,
          {
            file: "src/components/Toast.tsx",
            component: "Toast",
            symbol: "Toast.DismissButton",
          },
          "live nested button should retain source provenance",
        );

        const badge = ui.root?.children.find((node) => node.text === "Ready");
        expect(badge?.source).toMatchObject({
          raw: "src/components/Badge.tsx:17",
          file: "src/components/Badge.tsx",
          line: 17,
        });

        const malformed = ui.root?.children.find(
          (node) => node.text === "Broken",
        );
        expect(malformed?.source).toMatchObject({
          raw: "src/components/Broken.tsx:line",
          parseError: "invalid source line or column",
        });

        const plain = ui.root?.children.find(
          (node) => node.text === "No source",
        );
        expect(plain?.source).toBeUndefined();
      } finally {
        await new Promise<void>((resolve, reject) => {
          server.close((error) => {
            if (error) {
              reject(error);
              return;
            }
            resolve();
          });
        });
      }
    },
  );
});
