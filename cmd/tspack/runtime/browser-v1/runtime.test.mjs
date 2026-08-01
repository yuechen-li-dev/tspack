import assert from "node:assert/strict";
import { createServer } from "node:http";
import { readFile } from "node:fs/promises";
import { createRequire } from "node:module";
import { dirname, extname, join } from "node:path";
import test from "node:test";
import { fileURLToPath } from "node:url";

const require = createRequire(import.meta.url);
const { chromium } = require("playwright");
const runtimeDirectory = dirname(fileURLToPath(import.meta.url));

test("the canonical browser runtime preserves attachment, adapter, frame, and shutdown behavior", async (context) => {
  const server = await startRuntimeServer();
  const browser = await chromium.launch();
  context.after(async () => {
    await browser.close();
    await server.close();
  });

  const page = await browser.newPage();
  await page.goto(`${server.url}/`);
  const result = await page.evaluate(async runtimeURL => {
    const runtime = await import(runtimeURL);
    const waitFor = async (predicate, description, timeout = 2500) => {
      const deadline = performance.now() + timeout;
      while (performance.now() < deadline) {
        if (predicate()) return;
        await new Promise(resolve => setTimeout(resolve, 20));
      }
      throw new Error("Timed out waiting for " + description);
    };
    const plan = (attachmentId, componentInstanceId, payload, parentComponentInstanceId) => ({
      attachmentId,
      componentInstanceId,
      parentComponentInstanceId,
      hostBoxId: "Page.host",
      hostSelector: "[data-machina-box='host']",
      adapterId: "CustomElement",
      lifecycle: { mount: true, update: true, unmount: true },
      payload: { tagName: "copeland-widget", label: payload },
    });
    const artifact = plans => ({ schemaVersion: 1, projectId: "runtime-test", plans });

    const host = document.createElement("div");
    host.dataset.machinaBox = "host";
    document.body.appendChild(host);

    const directError = (() => {
      try {
        runtime.attachRenderer("missing", "component", "#missing", "CustomElement", "copeland-widget", "value");
      } catch (error) {
        return error.message;
      }
      return null;
    })();

    const runtimeDiagnostics = [];
    const originalConsoleError = console.error;
    console.error = (...values) => runtimeDiagnostics.push(values.map(value => String(value)).join(" "));
    runtime.registerAttachmentPlans(artifact([{
      ...plan("missing-host", "component", "never-mounted"),
      hostSelector: "[data-machina-box='missing']",
    }]));
    await waitFor(
      () => runtimeDiagnostics.some(message => message.includes("COPE-ATTACHMENT-PLAN-1007")),
      "bounded missing-host diagnostic",
      6000,
    );
    console.error = originalConsoleError;

    const rootPlan = plan("root", "component", "first");
    runtime.registerAttachmentPlans(artifact([rootPlan]));
    await waitFor(() => host.querySelector("[data-copeland-attachment='root']") !== null, "initial attachment mount");

    runtime.registerAttachmentPlans(artifact([plan("root", "component", "updated")]));
    await waitFor(() => host.querySelector("copeland-widget")?.getAttribute("label") === "updated", "compatible payload update");

    const replacementHost = document.createElement("div");
    replacementHost.dataset.machinaBox = "host";
    host.replaceWith(replacementHost);
    await waitFor(() => replacementHost.querySelector("[data-copeland-attachment='root']") !== null, "semantic host replacement recovery");

    runtime.registerAttachmentPlans(artifact([
      plan("parent", "parent", "parent"),
      plan("child", "child", "child", "parent"),
    ]));
    await waitFor(() => replacementHost.querySelectorAll("copeland-widget").length === 2, "parent and child attachments");
    runtime.registerAttachmentPlans(artifact([plan("parent", "parent", "parent")]));
    await waitFor(() => replacementHost.querySelectorAll("copeland-widget").length === 1, "child attachment teardown");

    const framePlan = plan("frame", "frame", "off");
    runtime.registerAttachmentPlans(artifact([framePlan]));
    await waitFor(() => replacementHost.querySelector("[data-copeland-attachment='frame']") !== null, "frame attachment mount");
    runtime.recordLegacyComponentFrameContract({ path: "fixtures/browser-v1-legacy/component-frames.js" });
    runtime.registerComponentFrames([{
      componentInstanceId: "frame",
      componentDefinitionId: "Toggle",
      stateIdentity: "ToggleState",
      initialState: { child: false },
      attachmentIds: ["frame"],
      eventContracts: {
        toggle: { payload: "void", transition: (_payload, state) => ({ child: !state.child }) },
      },
      project: state => ({
        plans: [plan("frame", "frame", state.child ? "on" : "off")],
        frames: state.child ? [{
          componentInstanceId: "frame-child",
          componentDefinitionId: "Child",
          stateIdentity: "ChildState",
          initialState: {},
          parentComponentInstanceId: "frame",
          attachmentIds: ["frame-child"],
          eventContracts: {},
          project: () => [],
          plans: [plan("frame-child", "frame-child", "child", "frame")],
        }] : [],
      }),
    }]);
    runtime.dispatchComponentEvent("frame", "toggle");
    await waitFor(() => runtime.inspectComponentFrame("frame-child") !== null, "projected child frame registration");
    await waitFor(() => replacementHost.querySelector("[data-copeland-attachment='frame-child']") !== null, "projected child mount");
    runtime.dispatchComponentEvent("frame", "toggle");
    await waitFor(() => runtime.inspectComponentFrame("frame-child") === null, "projected child frame teardown");

    runtime.destroyComponentFrame("frame");
    const destroyedFrameError = (() => {
      try {
        runtime.dispatchComponentEvent("frame", "toggle");
      } catch (error) {
        return error.message;
      }
      return null;
    })();

    const duplicateAdapterError = (() => {
      try {
        runtime.registerRendererAdapter("CustomElement", { mount() {}, update() {}, unmount() {} });
      } catch (error) {
        return error.message;
      }
      return null;
    })();

    const v1Plan = plan("v1-frame", "v1-frame", "before");
    runtime.registerAttachmentPlans(artifact([v1Plan]));
    await waitFor(() => replacementHost.querySelector("[data-copeland-attachment='v1-frame']") !== null, "V1 frame attachment mount");
    runtime.registerComponentFrameEnvelope({
      schemaVersion: 1,
      projectId: "runtime-test",
      frameDefinitions: [{
        frameDefinitionId: "v1-definition",
        componentDefinitionId: "V1Toggle",
        stateIdentity: "V1ToggleState",
        attachmentIds: ["v1-frame"],
        events: [{
          eventId: "v1-toggle",
          name: "Confirm",
          payloadContract: "void",
          transition: { kind: "constant", nextState: "after" },
        }],
        rendererEventName: "Confirm",
        presentationBranches: [],
        source: { path: "src/Test.ts", line: 1, column: 1 },
      }],
      frameInstances: [{
        componentInstanceId: "v1-frame",
        frameDefinitionId: "v1-definition",
        parentComponentInstanceId: null,
        initialState: "before",
        source: { path: "src/Test.ts", line: 1, column: 1 },
      }],
    });
    runtime.dispatchComponentEvent("v1-frame", "Confirm");
    await waitFor(() => replacementHost.querySelector("[data-copeland-attachment='v1-frame']")?.getAttribute("label") === "after", "V1 transition execution");
    const unsupportedEnvelopeError = (() => {
      try {
        runtime.registerComponentFrameEnvelope({ schemaVersion: 2, projectId: "runtime-test", frameDefinitions: [], frameInstances: [] });
      } catch (error) {
        return error.message;
      }
      return null;
    })();

    runtime.shutdownAttachmentPlans();
    runtime.shutdownComponentFrames();
    return {
      directError,
      runtimeDiagnostics,
      duplicateAdapterError,
      unsupportedEnvelopeError,
      destroyedFrameError,
      customElementRegistered: runtime.hasRendererAdapter("CustomElement"),
      root: runtime.inspectAttachmentRuntime("root"),
      child: runtime.inspectAttachmentRuntime("child"),
      frame: runtime.inspectAttachmentRuntime("frame"),
      trace: runtime.inspectComponentFrameTrace(),
      remaining: replacementHost.querySelectorAll("[data-copeland-attachment]").length,
    };
  }, `${server.url}/index.js`);

  assert.match(result.directError, /COPE-RENDERER-0007.*attachment=missing.*host=#missing/);
  assert.match(result.runtimeDiagnostics.join("\n"), /COPE-ATTACHMENT-PLAN-1007.*semantic host never appeared/);
  assert.match(result.duplicateAdapterError, /COPE-RENDERER-0013/);
  assert.match(result.unsupportedEnvelopeError, /COPE-COMPONENT-STATE-V1-1002/);
  assert.match(result.destroyedFrameError, /COPE-COMPONENT-STATE-0103/);
  assert.equal(result.customElementRegistered, true);
  assert.deepEqual(result.root, { mounts: 2, updates: 1, unmounts: 2, mounted: false, pending: false });
  assert.deepEqual(result.child, { mounts: 1, updates: 0, unmounts: 1, mounted: false, pending: false });
  assert.equal(result.frame.mounted, false);
  assert.equal(result.remaining, 0);
  assert.ok(result.trace.some(entry => entry.kind === "ChildFrameCreated"));
  assert.ok(result.trace.some(entry => entry.kind === "ChildFrameDestroyed"));
  assert.deepEqual(result.trace.filter(entry => entry.kind === "LegacyFrameContractLoaded").map(entry => entry.artifactPath), ["fixtures/browser-v1-legacy/component-frames.js"]);
});

async function startRuntimeServer() {
  const server = createServer(async (request, response) => {
    if (request.url === "/") {
      response.writeHead(200, { "Content-Type": "text/html" });
      response.end("<!doctype html><title>browser runtime test</title>");
      return;
    }
    const requestedPath = request.url === "/index.js" ? "index.js" : "";
    if (requestedPath === "") {
      response.writeHead(404);
      response.end();
      return;
    }
    try {
      const contents = await readFile(join(runtimeDirectory, requestedPath));
      response.writeHead(200, { "Content-Type": contentType(requestedPath) });
      response.end(contents);
    } catch {
      response.writeHead(500);
      response.end();
    }
  });
  await new Promise((resolve, reject) => {
    server.once("error", reject);
    server.listen(0, "127.0.0.1", resolve);
  });
  const address = server.address();
  return {
    close: () => new Promise(resolve => server.close(resolve)),
    url: `http://127.0.0.1:${address.port}`,
  };
}

function contentType(path) {
  return extname(path) === ".js" ? "text/javascript" : "application/octet-stream";
}
