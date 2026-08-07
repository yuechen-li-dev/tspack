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
  assert.strictEqual(installer.parseSemver("v0.1.8", "test").text, "v0.1.8");
  assert.throws(() => installer.parseSemver("latest", "test"), /semantic version/);
  assert.strictEqual(installer.compareSemver(installer.parseSemver("v0.1.8", "a"), installer.parseSemver("v0.1.7", "b")), 1);
}

function testMinimumVersionFile() {
  const root = fs.mkdtempSync(path.join(os.tmpdir(), "setup-tspack-version-"));
  fs.writeFileSync(path.join(root, ".tspack-version"), "v0.1.8\n");
  const requirement = installer.readMinimumVersion(root, ".tspack-version");
  assert.strictEqual(requirement.text, "v0.1.8");
  installer.enforceMinimumVersion("v0.1.8", requirement);
  installer.enforceMinimumVersion("v0.2.0", requirement);
  assert.throws(() => installer.enforceMinimumVersion("v0.1.7", requirement), /TSPACK_VERSION_TOO_OLD/);
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
  testMinimumVersionFile();
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
