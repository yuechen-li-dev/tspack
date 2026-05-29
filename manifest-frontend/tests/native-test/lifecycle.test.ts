import fs from "node:fs";
import os from "node:os";
import path from "node:path";
import { describe, expect, it } from "vitest";
import { runNativeTestFiles } from "../../src/native-test";
import { lifecycle } from "../../src/native-test/lifecycle";

function makeDir(): string {
  return fs.mkdtempSync(path.join(os.tmpdir(), "tspack-lifecycle-test-"));
}

function writePackageScript(root: string, scriptName: string, source: string): string {
  fs.mkdirSync(root, { recursive: true });
  const scriptPath = path.join(root, scriptName);
  fs.writeFileSync(scriptPath, source);
  return scriptPath;
}

function codes(result: { violations: Array<{ code: string }> }): string[] {
  return result.violations.map((violation) => violation.code);
}

describe("lifecycle behavior harness", () => {
  it("allows valid package and temp writes while capturing output and exit code", async () => {
    const packageDir = makeDir();
    writePackageScript(
      packageDir,
      "install.js",
      `
        const fs = require('node:fs');
        const path = require('node:path');
        fs.mkdirSync(path.join(process.cwd(), 'build'), { recursive: true });
        fs.writeFileSync(path.join(process.cwd(), 'build', 'output.txt'), 'ok');
        fs.writeFileSync(path.join(process.env.TMPDIR, 'scratch.txt'), 'tmp');
        fs.readFileSync(path.join(process.cwd(), 'build', 'output.txt'), 'utf8');
        console.log('valid stdout');
        console.error('valid stderr');
      `,
    );

    const result = await lifecycle.runScript({
      packageDir,
      command: "node ./install.js extra-arg",
    });

    expect(result.exitCode).toBe(0);
    expect(result.stdout).toContain("valid stdout");
    expect(result.stderr).toContain("valid stderr");
    expect(result.violations).toEqual([]);
    expect(result.writes.some((entry) => entry.endsWith(path.join("build", "output.txt")))).toBe(true);
    expect(fs.readFileSync(path.join(packageDir, "build", "output.txt"), "utf8")).toBe("ok");
  });

  it("denies network, child process, secret env, outside writes, and outside reads", async () => {
    const cases = [
      {
        name: "https",
        source: "require('node:https').get('https://example.invalid')",
        expected: "TSPACK_LIFECYCLE_NETWORK_DENIED",
      },
      {
        name: "net",
        source: "require('node:net').connect(443, 'example.invalid')",
        expected: "TSPACK_LIFECYCLE_NETWORK_DENIED",
      },
      {
        name: "child",
        source: "require('node:child_process').exec('echo bad')",
        expected: "TSPACK_LIFECYCLE_CHILD_PROCESS_DENIED",
      },
      {
        name: "env",
        source: "if (process.env.NPM_TOKEN) { console.log('visible'); }",
        expected: "TSPACK_LIFECYCLE_ENV_DENIED",
        exitCode: 0,
        env: { NPM_TOKEN: "sentinel-token" },
      },
      {
        name: "outside-write",
        source: "require('node:fs').writeFileSync('../outside.txt', 'bad')",
        expected: "TSPACK_LIFECYCLE_FS_WRITE_DENIED",
      },
      {
        name: "outside-read",
        source: "require('node:fs').readFileSync('../outside-secret.txt', 'utf8')",
        expected: "TSPACK_LIFECYCLE_FS_READ_DENIED",
      },
    ];

    for (const testCase of cases) {
      const root = makeDir();
      const packageDir = path.join(root, "package");
      fs.mkdirSync(packageDir);
      fs.writeFileSync(path.join(root, "outside-secret.txt"), "secret");
      writePackageScript(packageDir, "install.js", testCase.source);

      const result = await lifecycle.runScript({
        packageDir,
        command: "node install.js",
        env: testCase.env,
      });

      expect(codes(result), testCase.name).toContain(testCase.expected);
      if (testCase.exitCode !== undefined) {
        expect(result.exitCode).toBe(testCase.exitCode);
      }
    }
  });

  it("rejects arbitrary shell commands before execution", async () => {
    const packageDir = makeDir();
    writePackageScript(packageDir, "install.js", "console.log('must not run')");

    const result = await lifecycle.runScript({
      packageDir,
      command: "node install.js && curl https://example.invalid",
    });

    expect(result.exitCode).toBeNull();
    expect(result.stdout).toBe("");
    expect(codes(result)).toContain("TSPACK_LIFECYCLE_UNSUPPORTED_COMMAND");
  });

  it("scrubs inherited secrets and uses temporary HOME unless explicitly injected", async () => {
    const previousToken = process.env.NPM_TOKEN;
    process.env.NPM_TOKEN = "parent-secret";

    try {
      const packageDir = makeDir();
      writePackageScript(
        packageDir,
        "install.js",
        `
          if (process.env.NPM_TOKEN !== undefined) {
            throw new Error('parent secret leaked');
          }
          if (!process.env.HOME || process.env.HOME === ${JSON.stringify(os.homedir())}) {
            throw new Error('HOME was not isolated');
          }
          console.log(process.env.HOME);
        `,
      );

      const result = await lifecycle.runScript({
        packageDir,
        command: "node install.js",
        policy: { denyEnv: [] },
      });

      expect(result.exitCode).toBe(0);
      expect(result.stdout).toContain("tspack-lifecycle-");
      expect(result.violations).toEqual([]);
    } finally {
      if (previousToken === undefined) {
        delete process.env.NPM_TOKEN;
      } else {
        process.env.NPM_TOKEN = previousToken;
      }
    }
  });

  it("can run a lifecycle script detected in package metadata through xTest imports", async () => {
    const root = makeDir();
    const fixture = path.join(root, "fixture");
    const packageDir = path.join(fixture, "package");
    fs.mkdirSync(packageDir, { recursive: true });
    fs.writeFileSync(
      path.join(packageDir, "package.json"),
      JSON.stringify({ scripts: { postinstall: "node install.js" } }, null, 2),
    );
    fs.writeFileSync(
      path.join(packageDir, "install.js"),
      "require('node:fs').writeFileSync('postinstall.txt', 'ok')",
    );

    const importPath = path.resolve(process.cwd(), "src/native-test/index.ts").split(path.sep).join("/");
    fs.writeFileSync(
      path.join(root, "lifecycle.xtest.tsx"),
      `
        import { Suite, Fact, Project, assert, lifecycle } from '${importPath}';
        export default (<Suite name="lifecycle">
          <Fact name="explicit harness only"><Project from="${fixture.split(path.sep).join("/")}" />{async ({ project }) => {
            const metadata = await project.readJson('package/package.json');
            assert.equal(metadata.scripts.postinstall, 'node install.js', 'fixture exposes postinstall metadata');
            const result = await lifecycle.runScript({
              packageDir: project.path('package'),
              command: metadata.scripts.postinstall,
            });
            assert.equal(result.violations.map((entry) => entry.code), [], 'fixture lifecycle stays inside policy');
            assert.equal(result.exitCode, 0, 'lifecycle exit is preserved');
          }}</Fact>
        </Suite>);
      `,
    );

    const result = await runNativeTestFiles({ rootDir: root });
    expect(result.results).toHaveLength(1);
    expect(result.results[0].status).toBe("passed");
  });
});
