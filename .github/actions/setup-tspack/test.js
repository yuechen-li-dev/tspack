const assert = require("assert");
const crypto = require("crypto");
const fs = require("fs");
const os = require("os");
const path = require("path");

const installer = require("./index.js");

function testPlatformMapping() {
  assert.strictEqual(installer.mapPlatform("linux", "x64").artifact, "tspack-linux-amd64.tar.gz");
  assert.strictEqual(installer.mapPlatform("linux", "arm64").artifact, "tspack-linux-arm64.tar.gz");
  assert.strictEqual(installer.mapPlatform("darwin", "x64").artifact, "tspack-darwin-amd64.tar.gz");
  assert.strictEqual(installer.mapPlatform("darwin", "arm64").artifact, "tspack-darwin-arm64.tar.gz");
  assert.strictEqual(installer.mapPlatform("win32", "x64").artifact, "tspack-windows-amd64.zip");
  assert.throws(() => installer.mapPlatform("win32", "arm64"), /Windows arm64/);
}

function testChecksums() {
  const file = path.join(fs.mkdtempSync(path.join(os.tmpdir(), "setup-tspack-test-")), "archive.tar.gz");
  fs.writeFileSync(file, "archive contents");

  const digest = crypto.createHash("sha256").update("archive contents").digest("hex");
  const checksums = installer.parseChecksums(`${digest}  archive.tar.gz\n`);
  assert.strictEqual(checksums.get("archive.tar.gz"), digest);
  assert.strictEqual(installer.verifyChecksum(file, `${digest}  archive.tar.gz\n`, "archive.tar.gz"), digest);
  assert.throws(() => installer.verifyChecksum(file, `${"0".repeat(64)}  archive.tar.gz\n`, "archive.tar.gz"), /Checksum mismatch/);
  assert.throws(() => installer.verifyChecksum(file, `${digest}  other.tar.gz\n`, "archive.tar.gz"), /did not include/);
}

function testVersionParsing() {
  assert.strictEqual(installer.parseLatestTagName('{"tag_name":"v0.0.0-test"}'), "v0.0.0-test");
  assert.throws(() => installer.parseLatestTagName('{"name":"missing"}'), /tag_name/);
}

async function testResolveVersion() {
  assert.strictEqual(await installer.resolveVersion("v0.1.0", "owner/repo", ""), "v0.1.0");
}

function testUrls() {
  const urls = installer.buildDownloadUrls("test/tspack", "v0.0.0-test", "tspack-linux-amd64.tar.gz");
  assert.strictEqual(urls.artifactUrl, "https://github.com/test/tspack/releases/download/v0.0.0-test/tspack-linux-amd64.tar.gz");
  assert.strictEqual(urls.checksumsUrl, "https://github.com/test/tspack/releases/download/v0.0.0-test/checksums.txt");
}

function testFindExtractedBinary() {
  const root = fs.mkdtempSync(path.join(os.tmpdir(), "setup-tspack-find-"));
  const nested = path.join(root, "tspack-v0.0.0", "bin");
  fs.mkdirSync(nested, { recursive: true });
  const binary = path.join(nested, "tspack");
  fs.writeFileSync(binary, "#!/bin/sh\n");
  assert.strictEqual(installer.findExtractedBinary(root, "tspack"), binary);
}

function testCheckInput() {
  assert.strictEqual(installer.shouldRunCheck("true"), true);
  assert.strictEqual(installer.shouldRunCheck("false"), false);
  assert.strictEqual(installer.shouldRunCheck("0"), false);
}

async function main() {
  testPlatformMapping();
  testChecksums();
  testVersionParsing();
  await testResolveVersion();
  testUrls();
  testFindExtractedBinary();
  testCheckInput();
  console.log("setup-tspack tests passed");
}

main().catch((error) => {
  console.error(error);
  process.exit(1);
});
