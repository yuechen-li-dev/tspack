import { spawn } from "node:child_process";
import fsSync from "node:fs";
import fs from "node:fs/promises";
import os from "node:os";
import path from "node:path";
import { fileURLToPath } from "node:url";

export type LifecycleViolation = {
  code: string;
  kind: string;
  detail: string;
  path?: string;
  module?: string;
  envKey?: string;
};

export type LifecycleProbePolicy = {
  denyNetwork?: boolean;
  denyChildProcess?: boolean;
  denyEnv?: string[];
  allowRead?: string[];
  allowWrite?: string[];
};

export type LifecycleRunScriptRequest = {
  packageDir: string;
  command: string;
  script?: string;
  policy?: LifecycleProbePolicy;
  env?: Record<string, string>;
  timeoutSeconds?: number;
};

export type LifecycleRunScriptResult = {
  exitCode: number | null;
  signal?: string | null;
  timedOut: boolean;
  stdout: string;
  stderr: string;
  violations: LifecycleViolation[];
  reads: string[];
  writes: string[];
};

type ParsedNodeCommand = {
  scriptPath: string;
  args: string[];
};

type GuardReport = {
  violations?: LifecycleViolation[];
  reads?: string[];
  writes?: string[];
  guardFailures?: string[];
};

const commonDeniedEnvKeys = [
  "NPM_TOKEN",
  "NODE_AUTH_TOKEN",
  "GITHUB_TOKEN",
  "GITHUB_ACTIONS",
  "AWS_ACCESS_KEY_ID",
  "AWS_SECRET_ACCESS_KEY",
  "AWS_SESSION_TOKEN",
  "VAULT_TOKEN",
  "SSH_AUTH_SOCK",
  "GOOGLE_APPLICATION_CREDENTIALS",
  "AZURE_CLIENT_SECRET",
];

const defaultPolicy: Required<LifecycleProbePolicy> = {
  denyNetwork: true,
  denyChildProcess: true,
  denyEnv: commonDeniedEnvKeys,
  allowRead: ["package/**", "tmp/**"],
  allowWrite: ["package/**", "tmp/**"],
};

export const lifecycle = {
  runScript,
};

export async function runScript(
  request: LifecycleRunScriptRequest,
): Promise<LifecycleRunScriptResult> {
  const packageDir = path.resolve(request.packageDir);
  const parsed = parseNodeLifecycleCommand(request.command);
  if (!parsed) {
    return unsupportedCommandResult(request.command);
  }

  const scriptPath = path.resolve(packageDir, parsed.scriptPath);
  if (!isInsideDirectory(scriptPath, packageDir)) {
    return unsupportedCommandResult(
      `lifecycle script escapes package directory: ${parsed.scriptPath}`,
    );
  }

  const tempRoot = await fs.mkdtemp(path.join(os.tmpdir(), "tspack-lifecycle-"));
  const tempHome = path.join(tempRoot, "home");
  const tempDir = path.join(tempRoot, "tmp");
  const reportPath = path.join(tempRoot, "guard-report.json");
  const configPath = path.join(tempRoot, "guard-config.json");

  try {
    await fs.mkdir(tempHome, { recursive: true });
    await fs.mkdir(tempDir, { recursive: true });

    const policy = normalizePolicy(request.policy);
    const config = {
      packageDir,
      tempDir,
      denyNetwork: policy.denyNetwork,
      denyChildProcess: policy.denyChildProcess,
      denyEnv: policy.denyEnv,
      allowReadRoots: resolvePolicyRoots(policy.allowRead, packageDir, tempDir),
      allowWriteRoots: resolvePolicyRoots(policy.allowWrite, packageDir, tempDir),
    };

    await fs.writeFile(configPath, JSON.stringify(config, null, 2), "utf8");

    const env = createScrubbedEnvironment(tempHome, tempDir, request.env);
    env.TSPACK_LIFECYCLE_GUARD_CONFIG = configPath;
    env.TSPACK_LIFECYCLE_GUARD_REPORT = reportPath;

    const guardPath = resolveGuardPath();
    const args = [
      "--require",
      guardPath,
      scriptPath,
      ...parsed.args,
    ];
    const spawned = await spawnAndCapture(
      process.argv[0],
      args,
      packageDir,
      env,
      request.timeoutSeconds ?? 30,
    );
    const reportResult = await readGuardReport(reportPath, packageDir);

    return {
      exitCode: spawned.exitCode,
      signal: spawned.signal,
      timedOut: spawned.timedOut,
      stdout: spawned.stdout,
      stderr: spawned.stderr,
      violations: reportResult.violations,
      reads: reportResult.reads,
      writes: reportResult.writes,
    };
  } finally {
    await fs.rm(tempRoot, { recursive: true, force: true });
  }
}

function normalizePolicy(
  policy: LifecycleProbePolicy | undefined,
): Required<LifecycleProbePolicy> {
  return {
    denyNetwork: policy?.denyNetwork ?? defaultPolicy.denyNetwork,
    denyChildProcess:
      policy?.denyChildProcess ?? defaultPolicy.denyChildProcess,
    denyEnv: policy?.denyEnv ?? defaultPolicy.denyEnv,
    allowRead: policy?.allowRead ?? defaultPolicy.allowRead,
    allowWrite: policy?.allowWrite ?? defaultPolicy.allowWrite,
  };
}

function parseNodeLifecycleCommand(command: string): ParsedNodeCommand | undefined {
  const tokens = tokenizeCommand(command);
  if (!tokens || tokens.length < 2) {
    return undefined;
  }

  const executable = tokens[0];
  if (executable !== "node") {
    return undefined;
  }

  const scriptPath = tokens[1];
  if (
    !scriptPath ||
    scriptPath.startsWith("-") ||
    path.isAbsolute(scriptPath)
  ) {
    return undefined;
  }

  if (scriptPath.includes("..") || scriptPath.includes("\\")) {
    return undefined;
  }

  return {
    scriptPath,
    args: tokens.slice(2),
  };
}

function tokenizeCommand(command: string): string[] | undefined {
  if (typeof command !== "string" || command.trim().length === 0) {
    return undefined;
  }

  const tokens: string[] = [];
  let current = "";
  let quote: string | undefined;

  for (let index = 0; index < command.length; index += 1) {
    const char = command[index];

    if (!quote && isShellMetacharacter(char)) {
      return undefined;
    }

    if ((char === "'" || char === '"') && !quote) {
      quote = char;
      continue;
    }

    if (quote === char) {
      quote = undefined;
      continue;
    }

    if (!quote && /\s/.test(char)) {
      if (current.length > 0) {
        tokens.push(current);
        current = "";
      }
      continue;
    }

    current += char;
  }

  if (quote) {
    return undefined;
  }

  if (current.length > 0) {
    tokens.push(current);
  }

  return tokens;
}

function isShellMetacharacter(char: string): boolean {
  return ["&", "|", ";", "<", ">", "`", "$", "(", ")"].includes(char);
}

function unsupportedCommandResult(detail: string): LifecycleRunScriptResult {
  return {
    exitCode: null,
    signal: null,
    timedOut: false,
    stdout: "",
    stderr: "",
    violations: [
      {
        code: "TSPACK_LIFECYCLE_UNSUPPORTED_COMMAND",
        kind: "command",
        detail,
      },
    ],
    reads: [],
    writes: [],
  };
}

function createScrubbedEnvironment(
  tempHome: string,
  tempDir: string,
  overlay: Record<string, string> | undefined,
): Record<string, string> {
  const env: Record<string, string> = {};
  copyEnvKey(env, "PATH");
  copyEnvKey(env, "Path");
  copyEnvKey(env, "SystemRoot");
  copyEnvKey(env, "WINDIR");

  env.HOME = tempHome;
  env.TMPDIR = tempDir;
  env.TEMP = tempDir;
  env.TMP = tempDir;

  if (overlay) {
    for (const [key, value] of Object.entries(overlay)) {
      env[key] = value;
    }
  }

  return env;
}

function copyEnvKey(target: Record<string, string>, key: string): void {
  const value = process.env[key];
  if (typeof value === "string") {
    target[key] = value;
  }
}

function resolvePolicyRoots(
  patterns: string[],
  packageDir: string,
  tempDir: string,
): string[] {
  const roots: string[] = [];

  for (const pattern of patterns) {
    const root = resolvePolicyRoot(pattern, packageDir, tempDir);
    if (root) {
      roots.push(root);
    }
  }

  return roots;
}

function resolvePolicyRoot(
  pattern: string,
  packageDir: string,
  tempDir: string,
): string | undefined {
  if (pattern === "package" || pattern === "package/**") {
    return packageDir;
  }
  if (pattern === "tmp" || pattern === "tmp/**") {
    return tempDir;
  }
  if (pattern.startsWith("package/")) {
    const relative = pattern.slice("package/".length).replace(/\/\*\*$/, "");
    return path.resolve(packageDir, relative);
  }
  if (pattern.startsWith("tmp/")) {
    const relative = pattern.slice("tmp/".length).replace(/\/\*\*$/, "");
    return path.resolve(tempDir, relative);
  }
  return undefined;
}

function isInsideDirectory(filePath: string, directory: string): boolean {
  const relative = path.relative(directory, filePath);
  return (
    relative === "" ||
    (!relative.startsWith("..") && !path.isAbsolute(relative))
  );
}

async function spawnAndCapture(
  executable: string,
  args: string[],
  cwd: string,
  env: Record<string, string>,
  timeoutSeconds: number,
): Promise<{
  exitCode: number | null;
  signal: string | null;
  timedOut: boolean;
  stdout: string;
  stderr: string;
}> {
  return await new Promise((resolve) => {
    let stdout = "";
    let stderr = "";
    let timedOut = false;
    let settled = false;

    const child = spawn(executable, args, {
      cwd,
      env,
      stdio: ["ignore", "pipe", "pipe"],
    }) as ReturnType<typeof spawn> & {
      on(event: "error", listener: (error: Error) => void): void;
      on(
        event: "close",
        listener: (exitCode: number | null, signal: string | null) => void,
      ): void;
    };

    child.stdout?.on("data", (chunk) => {
      stdout += String(chunk);
    });
    child.stderr?.on("data", (chunk) => {
      stderr += String(chunk);
    });

    const timer = setTimeout(() => {
      timedOut = true;
      child.kill("SIGKILL");
    }, Math.max(1, timeoutSeconds * 1000));

    child.on("error", (error) => {
      if (settled) {
        return;
      }
      settled = true;
      clearTimeout(timer);
      stderr += error.message;
      resolve({ exitCode: null, signal: null, timedOut, stdout, stderr });
    });

    child.on("close", (exitCode, signal) => {
      if (settled) {
        return;
      }
      settled = true;
      clearTimeout(timer);
      resolve({ exitCode, signal, timedOut, stdout, stderr });
    });
  });
}

async function readGuardReport(
  reportPath: string,
  packageDir: string,
): Promise<{
  violations: LifecycleViolation[];
  reads: string[];
  writes: string[];
}> {
  try {
    const raw = await fs.readFile(reportPath, "utf8");
    const report = JSON.parse(raw) as GuardReport;
    const violations = Array.isArray(report.violations)
      ? [...report.violations]
      : [];

    if (Array.isArray(report.guardFailures)) {
      for (const failure of report.guardFailures) {
        violations.push({
          code: "TSPACK_LIFECYCLE_GUARD_FAILED",
          kind: "guard",
          detail: String(failure),
        });
      }
    }

    return {
      violations,
      reads: Array.isArray(report.reads) ? report.reads.map(String) : [],
      writes: Array.isArray(report.writes) ? report.writes.map(String) : [],
    };
  } catch (error) {
    return {
      violations: [
        {
          code: "TSPACK_LIFECYCLE_REPORT_MISSING",
          kind: "guard",
          detail: (error as Error).message,
          path: reportPath,
        },
      ],
      reads: [],
      writes: [],
    };
  }
}

function resolveGuardPath(): string {
  const localPath = path.resolve(
    path.dirname(fileURLToPath(import.meta.url)),
    "lifecycle-guard.cjs",
  );
  if (fsSync.existsSync(localPath)) {
    return localPath;
  }

  return path.resolve(
    process.cwd(),
    "src",
    "native-test",
    "lifecycle-guard.cjs",
  );
}
