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
  inspect,
  runSuite,
  typecheckNativeTestFile,
  type InspectResult,
} from "../../src/native-test";
import type { InspectOptions as BackendInspectOptions } from "../../src/inspect/index";

async function chromiumAvailability(): Promise<{ available: boolean; reason?: string }> {
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
          bounds: { x: 20, y: 20, width: 80, height: 30 },
          visible: true,
          children: [],
        },
      ],
    },
    hitTests: [],
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
    ]);
    expect(inspect.findByRole(ui.root, "button", "Save")?.id).toBe("node-2");
    expect(inspect.findByText(ui.root, /Save/)?.id).toBe("node-1");
  });

  it("does not count inspect observation as a native test assertion", async () => {
    const helper = createInspectHelper(async () => sampleResult());
    const root = Suite({ name: "inspect" }, Fact({ name: "only observes" }, async () => {
      await helper.url("http://127.0.0.1:5173");
    }));

    const results = await runSuite(root, {});

    expect(results[0].status).toBe("failed");
    expect((results[0].error as { code?: string }).code).toBe(
      "TSPACK_TEST_NO_ASSERTION",
    );
  });

  it("allows inspect data to be asserted after observation", async () => {
    const helper = createInspectHelper(async () => sampleResult());
    const root = Suite({ name: "inspect" }, Fact({ name: "asserts" }, async () => {
      const ui = await helper.url("http://127.0.0.1:5173");
      assert.equal(ui.root?.role, "main", "main landmark should be present");
    }));

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
    const root = fs.mkdtempSync(path.join(os.tmpdir(), "tspack-inspect-types-"));
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
            const cdp = await inspect.cdp("http://127.0.0.1:9229", {
              target: 0,
              selector: ".statusbar",
              points: [{ x: 3, y: 4 }],
            });
            assert.type<string | undefined>(ui.root?.role, "url inspect root role is typed");
            assert.type<string>(cdp.target.url, "cdp inspect target url is typed");
          }}</Fact>
        </Suite>
      );
      `,
    );

    const result = typecheckNativeTestFile(filePath, { rootDir: root });

    expect(result.diagnostics).toEqual([]);
  });

  itWithChromium("can inspect a tiny local page when Playwright Chromium is available", async () => {
    const server = http.createServer((request, response) => {
      response.setHeader("content-type", "text/html");
      response.end(`<!doctype html><main role="main"><button>Save</button></main>`);
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
      expect(ui.root?.children.some((node) => node.role === "button")).toBe(
        true,
      );
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
  });
});
