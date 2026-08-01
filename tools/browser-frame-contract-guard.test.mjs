import assert from "node:assert/strict";
import { readFile } from "node:fs/promises";
import test from "node:test";
import { resolve } from "node:path";

const maintainedSources = [
  "../../Copeland/src/Copeland/Copeland.Cli/ComponentFrameArtifactEmitter.cs",
  "../../Copeland/samples/copeland-ts/copeland-website-m0/browser-proof.mjs",
  "../../Copeland/samples/copeland-ts/standalone-web-m0/frontend/generated/component-frames.js",
];

test("maintained frame producers and browser proofs do not use the legacy registration API", async () => {
  for (const source of maintainedSources) {
    const path = resolve(import.meta.dirname, source);
    const contents = await readFile(path, "utf8");
    assert.doesNotMatch(contents, /registerComponentFrames/);
  }
});

test("the explicit browser-v1 fixture remains the only legacy frame artifact", async () => {
  const path = resolve(import.meta.dirname, "../fixtures/browser-v1-legacy-component-frames/component-frames.js");
  const contents = await readFile(path, "utf8");
  assert.match(contents, /registerComponentFrames/);
  assert.doesNotMatch(contents, /schemaVersion:\s*1/);
});
