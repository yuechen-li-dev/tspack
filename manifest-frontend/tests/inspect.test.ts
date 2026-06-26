import { describe, expect, it } from "vitest";
import http from "node:http";
import crypto from "node:crypto";
import net from "node:net";
import fs from "node:fs";
import os from "node:os";
import path from "node:path";
import { formatInspectJson, formatInspectText } from "../src/inspect/format.js";
import {
  buildInspectFailureResult,
  parsePoint,
  parseViewport,
} from "../src/inspect/index.js";
import {
  buildInspectAnalyzerExpression,
  findVSCodeExecutable,
  findWindowsChromiumExecutable,
  resolveInspectBackend,
  resolveVSCodeElectronExecutable,
  runInspect,
} from "../src/inspect/backend.js";
import { listCdpTargets, normalizeCdpEndpoint } from "../src/inspect/cdp.js";

function decodeClientWebSocketTextFrame(
  buffer: Buffer,
): { payload: string; remaining: Buffer } | undefined {
  if (buffer.length < 6) {
    return undefined;
  }

  const opcode = buffer[0] & 0x0f;
  let payloadLength = buffer[1] & 0x7f;
  let offset = 2;
  if (payloadLength === 126) {
    if (buffer.length < 8) {
      return undefined;
    }
    payloadLength = buffer.readUInt16BE(offset);
    offset += 2;
  }

  const mask = buffer.subarray(offset, offset + 4);
  offset += 4;
  if (buffer.length < offset + payloadLength) {
    return undefined;
  }

  const data = Buffer.from(buffer.subarray(offset, offset + payloadLength));
  for (let index = 0; index < data.length; index += 1) {
    data[index] = data[index] ^ mask[index % 4];
  }

  const remaining = buffer.subarray(offset + payloadLength);
  if (opcode !== 1) {
    return { payload: "", remaining };
  }

  return {
    payload: data.toString("utf8"),
    remaining,
  };
}

function encodeServerWebSocketTextFrame(payload: string): Buffer {
  const data = Buffer.from(payload, "utf8");
  if (data.length < 126) {
    return Buffer.concat([Buffer.from([0x81, data.length]), data]);
  }

  if (data.length <= 0xffff) {
    const header = Buffer.from([
      0x81,
      126,
      (data.length >> 8) & 0xff,
      data.length & 0xff,
    ]);
    return Buffer.concat([header, data]);
  }

  throw new Error("test websocket payload too large");
}

async function createFakeCdpServer(): Promise<{
  endpoint: string;
  close: () => Promise<void>;
}> {
  let webSocketPath = "/devtools/browser/fake-browser";

  const server = http.createServer((req, res) => {
    if ((req.url ?? "").startsWith("/json/list")) {
      res.setHeader("content-type", "application/json");
      res.end("[]");
      return;
    }

    if ((req.url ?? "").startsWith("/json/version")) {
      const address = server.address();
      if (!address || typeof address === "string") {
        throw new Error("failed to read fake cdp server address");
      }
      res.setHeader("content-type", "application/json");
      res.end(
        JSON.stringify({
          webSocketDebuggerUrl: `ws://127.0.0.1:${address.port}${webSocketPath}`,
        }),
      );
      return;
    }

    res.statusCode = 404;
    res.end("missing");
  });

  server.on("upgrade", (req, socket) => {
    if (req.url !== webSocketPath) {
      socket.destroy();
      return;
    }

    const key = req.headers["sec-websocket-key"];
    if (typeof key !== "string") {
      socket.destroy();
      return;
    }

    const accept = crypto
      .createHash("sha1")
      .update(`${key}258EAFA5-E914-47DA-95CA-C5AB0DC85B11`)
      .digest("base64");
    socket.write(
      [
        "HTTP/1.1 101 Switching Protocols",
        "Upgrade: websocket",
        "Connection: Upgrade",
        `Sec-WebSocket-Accept: ${accept}`,
        "",
        "",
      ].join("\r\n"),
    );

    let frameBuffer = Buffer.alloc(0);
    socket.on("data", (chunk) => {
      frameBuffer = Buffer.concat([frameBuffer, chunk]);
      const decoded = decodeClientWebSocketTextFrame(frameBuffer);
      if (!decoded) {
        return;
      }
      frameBuffer = decoded.remaining;
      if (!decoded.payload) {
        return;
      }
      const command = JSON.parse(decoded.payload) as {
        id: number;
        method: string;
      };
      if (command.method !== "Target.getTargets") {
        socket.destroy();
        return;
      }

      socket.write(
        encodeServerWebSocketTextFrame(
          JSON.stringify({
            id: command.id,
            result: {
              targetInfos: [
                {
                  targetId: "vscode-workbench",
                  type: "page",
                  title: "Visual Studio Code",
                  url: "vscode-file://vscode-app/out/vs/code/electron-sandbox/workbench/workbench.html",
                },
                {
                  targetId: "service-worker",
                  type: "service_worker",
                  title: "ignored",
                  url: "vscode-file://ignored",
                },
              ],
            },
          }),
        ),
        () => socket.end(),
      );
    });
  });

  await new Promise<void>((resolve) =>
    server.listen(0, "127.0.0.1", () => resolve()),
  );
  const address = server.address();
  if (!address || typeof address === "string") {
    throw new Error("failed to bind fake cdp server");
  }

  return {
    endpoint: `http://127.0.0.1:${address.port}`,
    close: async () => {
      server.closeAllConnections();
      await new Promise<void>((resolve, reject) =>
        server.close((err) => (err ? reject(err) : resolve())),
      );
    },
  };
}

describe("inspect parsing", () => {
  it("parses viewport", () => {
    expect(parseViewport("1440x900")).toEqual({ width: 1440, height: 900 });
    expect(() => parseViewport("a")).toThrow("TSPACK_INSPECT_INVALID_VIEWPORT");
  });

  it("parses point", () => {
    expect(parsePoint("320,148")).toEqual({ x: 320, y: 148 });
    expect(() => parsePoint("-1,2")).toThrow("TSPACK_INSPECT_INVALID_POINT");
  });

  it("formats text and json", () => {
    const result = {
      target: { url: "http://localhost:5173" },
      browser: { name: "chromium" as const },
      viewport: { width: 1440, height: 900 },
      root: {
        id: "node-1",
        tag: "h1",
        role: "heading",
        name: "Title",
        bounds: { x: 1, y: 2, width: 3, height: 4 },
        visible: true,
        children: [],
      },
      hitTests: [],
      diagnostics: [],
    };
    const text = formatInspectText(result);
    expect(text).toContain("UI Inspect: http://localhost:5173");
    expect(text).toContain("Browser: chromium");
    const json = formatInspectJson(result);
    expect(() => JSON.parse(json)).not.toThrow();
    expect(json.endsWith("\n")).toBe(true);
  });

  it("routes ordinary URL inspect to Playwright Chromium without VS Code discovery", () => {
    const baseOptions = {
      browser: "auto" as const,
      viewport: { width: 800, height: 600 },
      points: [] as Array<{ x: number; y: number }>,
      json: true,
    };

    expect(
      resolveInspectBackend({
        ...baseOptions,
        url: "http://127.0.0.1:4171",
      }),
    ).toBe("playwright-chromium");

    expect(
      resolveInspectBackend({
        ...baseOptions,
        url: "https://example.test",
      }),
    ).toBe("playwright-chromium");

    expect(
      resolveInspectBackend({
        ...baseOptions,
        url: "http://127.0.0.1:4171",
        selector: "main",
        points: [{ x: 20, y: 30 }],
      }),
    ).toBe("playwright-chromium");
  });

  it("preserves explicit VS Code, platform-webview, host-path, browser-path, and CDP routing", () => {
    const baseOptions = {
      url: "http://127.0.0.1:4171",
      browser: "auto" as const,
      viewport: { width: 800, height: 600 },
      points: [] as Array<{ x: number; y: number }>,
      json: true,
    };

    expect(
      resolveInspectBackend({
        ...baseOptions,
        browser: "vscode",
      }),
    ).toBe("vscode");

    expect(
      resolveInspectBackend({
        ...baseOptions,
        browser: "platform-webview",
      }),
    ).toBe("platform-webview");

    expect(
      resolveInspectBackend({
        ...baseOptions,
        cdpEndpoint: "http://127.0.0.1:9222",
      }),
    ).toBe("cdp");

    expect(
      resolveInspectBackend({
        ...baseOptions,
        hostPath: "/tmp/host",
      }),
    ).toBe("host-path");

    expect(
      resolveInspectBackend({
        ...baseOptions,
        browserPath: "/tmp/browser",
      }),
    ).toBe("browser-path");
  });

  it("builds analyzer evaluate expressions with serialized selector and point args", () => {
    const expression = buildInspectAnalyzerExpression(
      '#root[aria-label=\"A B\"]',
      [{ x: 12, y: 34 }],
    );

    expect(expression).toContain(')({\"selector\":');
    expect(expression).toContain('\"#root[aria-label=\\\"A B\\\"]\"');
    expect(expression).toContain('\"points\":[{\"x\":12,\"y\":34}]');
    expect(expression).not.toContain("page.evaluate");
  });

  it("accepts WebKit backend aliases as real backend values", async () => {
    await expect(
      runInspect({
        url: "http://127.0.0.1:1/unreachable",
        browser: "webkit",
        viewport: { width: 800, height: 600 },
        points: [],
        json: true,
      }),
    ).rejects.toThrow(
      /TSPACK_INSPECT_(PAGE_LOAD_FAILED|BROWSER_LAUNCH_FAILED|BROWSER_NOT_FOUND)/,
    );

    await expect(
      runInspect({
        url: "http://127.0.0.1:1/unreachable",
        browser: "playwright-webkit",
        viewport: { width: 800, height: 600 },
        points: [],
        json: true,
      }),
    ).rejects.toThrow(
      /TSPACK_INSPECT_(PAGE_LOAD_FAILED|BROWSER_LAUNCH_FAILED|BROWSER_NOT_FOUND)/,
    );
  }, 15000);

  it("does not report VS Code discovery errors for ordinary auto URL inspect", async () => {
    try {
      await runInspect({
        url: "http://127.0.0.1:1/unreachable",
        browser: "auto",
        viewport: { width: 800, height: 600 },
        points: [],
        json: true,
      });
      throw new Error("expected inspect to fail for unreachable URL");
    } catch (error: unknown) {
      const message = error instanceof Error ? error.message : String(error);
      expect(message).not.toContain("TSPACK_INSPECT_VSCODE_NOT_FOUND");
      expect(message).toMatch(
        /TSPACK_INSPECT_(BROWSER_LAUNCH_FAILED|PAGE_LOAD_FAILED)/,
      );
    }
  }, 15000);

  it("supports discovery and browser selection errors", async () => {
    expect(
      findVSCodeExecutable() === null ||
        typeof findVSCodeExecutable() === "string",
    ).toBe(true);

    await expect(
      runInspect({
        url: "http://127.0.0.1:9999",
        browser: "browser-path",
        browserPath: "/definitely/missing/browser",
        viewport: { width: 800, height: 600 },
        points: [],
        json: true,
      }),
    ).rejects.toThrow("TSPACK_INSPECT_BROWSER_PATH_NOT_FOUND");

    await expect(
      runInspect({
        url: "http://127.0.0.1:9999",
        browser: "vscode",
        viewport: { width: 800, height: 600 },
        points: [],
        json: true,
      }),
    ).rejects.toThrow(/TSPACK_INSPECT_(VSCODE_|PAGE_LOAD_FAILED)/);

    await expect(
      runInspect({
        url: "http://127.0.0.1:9999",
        browser: "platform-webview",
        viewport: { width: 800, height: 600 },
        points: [],
        json: true,
      }),
    ).rejects.toThrow(
      /TSPACK_INSPECT_PLATFORM_WEBVIEW_(UNAVAILABLE|INIT_FAILED)/,
    );
  }, 15000);

  it("resolves vscode wrapper path to electron binary when available", () => {
    const root = fs.mkdtempSync(
      path.join(os.tmpdir(), "inspect-vscode-resolve-"),
    );
    const wrapperDir = path.join(root, "bin");
    const shareDir = path.join(root, "share", "code");
    fs.mkdirSync(wrapperDir, { recursive: true });
    fs.mkdirSync(shareDir, { recursive: true });
    const wrapperPath = path.join(wrapperDir, "code");
    const electronPath = path.join(shareDir, "code");
    fs.writeFileSync(wrapperPath, "#!/usr/bin/env bash\n", { mode: 0o755 });
    fs.writeFileSync(electronPath, "binary", { mode: 0o755 });

    const resolved = resolveVSCodeElectronExecutable(wrapperPath);
    if (process.platform === "win32") {
      expect(resolved).toBe(wrapperPath);
      return;
    }

    expect(resolved).toBe(electronPath);
  });

  it("discovers Windows Chromium candidates with space-containing paths", () => {
    const fakeEnv = {
      ProgramFiles: "C:\\Program Files",
      "ProgramFiles(x86)": "C:\\Program Files (x86)",
      LocalAppData: "C:\\Users\\me\\AppData\\Local",
    } as NodeJS.ProcessEnv;
    const seenPaths: string[] = [];
    const result = findWindowsChromiumExecutable(fakeEnv, (candidate) => {
      seenPaths.push(candidate);
      return (
        candidate ===
        "C:\\Program Files (x86)\\Microsoft\\Edge\\Application\\msedge.exe"
      );
    });

    expect(result).toBe(
      "C:\\Program Files (x86)\\Microsoft\\Edge\\Application\\msedge.exe",
    );
    expect(seenPaths.some((candidate) => candidate.includes("Program Files"))).toBe(
      true,
    );
  });

  it("builds JSON-friendly inspect failures with structured diagnostics", () => {
    const failure = buildInspectFailureResult(
      {
        url: "http://127.0.0.1:9",
        browser: "auto",
        viewport: { width: 1440, height: 900 },
        points: [],
        json: true,
      },
      new Error(
        "TSPACK_INSPECT_BROWSER_NOT_FOUND: Playwright Chromium browser is unavailable | Playwright Chromium browser runtime is not installed or could not be located. | Install the Playwright Chromium browser with: npx playwright install chromium",
      ),
    );

    expect(failure.target.url).toBe("http://127.0.0.1:9");
    expect(failure.browser.backend).toBe("playwright");
    expect(failure.browser.name).toBe("chromium");
    expect(failure.root).toBeNull();
    expect(failure.diagnostics[0].code).toBe("TSPACK_INSPECT_BROWSER_NOT_FOUND");
    expect(failure.diagnostics[0].details).toContain(
      "Playwright Chromium browser runtime is not installed or could not be located.",
    );
    expect(failure.diagnostics[0].fixes).toContain(
      "Install the Playwright Chromium browser with: npx playwright install chromium",
    );
    expect(() => JSON.parse(formatInspectJson(failure))).not.toThrow();
  });

  it("falls back to Target.getTargets when /json/list has no inspectable targets", async () => {
    const server = await createFakeCdpServer();
    try {
      const result = await listCdpTargets(server.endpoint);
      expect(result.command).toBe("inspect");
      expect(result.mode).toBe("list-targets");
      expect(result.cdp).toBe(server.endpoint);
      expect(result.targets).toHaveLength(1);
      expect(result.targets[0].id).toBe("vscode-workbench");
      expect(result.targets[0].url).toContain("vscode-file://vscode-app");
      expect(result.targets[0].webSocketDebuggerUrl).toContain(
        "/devtools/browser/fake-browser",
      );
      expect(result.diagnostics[0].code).toBe(
        "TSPACK_INSPECT_CDP_TARGETS_FROM_WEBSOCKET",
      );
    } finally {
      await server.close();
    }
  });
});

describe("inspect cdp helpers", () => {
  it("validates cdp endpoint", () => {
    expect(normalizeCdpEndpoint("http://127.0.0.1:9222")).toBe(
      "http://127.0.0.1:9222",
    );
    expect(() => normalizeCdpEndpoint("")).toThrow(
      "TSPACK_INSPECT_CDP_ENDPOINT_REQUIRED",
    );
    expect(() => normalizeCdpEndpoint("ws://127.0.0.1:9222")).toThrow(
      "TSPACK_INSPECT_CDP_ENDPOINT_INVALID",
    );
  });

  it("lists inspectable cdp targets", async () => {
    const server = http.createServer((req, res) => {
      if ((req.url ?? "").startsWith("/json/list")) {
        res.setHeader("content-type", "application/json");
        res.end(
          JSON.stringify([
            {
              id: "page-1",
              type: "page",
              title: "App",
              url: "http://localhost:5173/",
              webSocketDebuggerUrl: "ws://x/page-1",
            },
            {
              id: "devtools",
              type: "other",
              title: "DevTools",
              url: "devtools://devtools",
            },
          ]),
        );
        return;
      }
      res.statusCode = 404;
      res.end("missing");
    });
    await new Promise<void>((resolve) =>
      server.listen(0, "127.0.0.1", () => resolve()),
    );
    const address = server.address();
    if (!address || typeof address === "string") {
      throw new Error("failed to bind test server");
    }
    const endpoint = `http://127.0.0.1:${address.port}`;

    try {
      const result = await listCdpTargets(endpoint);
      expect(result.command).toBe("inspect");
      expect(result.mode).toBe("list-targets");
      expect(result.cdp).toBe(endpoint);
      expect(result.targets).toHaveLength(1);
      expect(result.targets[0].index).toBe(0);
      expect(result.targets[0].id).toBe("page-1");
    } finally {
      await new Promise<void>((resolve, reject) =>
        server.close((err) => (err ? reject(err) : resolve())),
      );
    }
  });
});
