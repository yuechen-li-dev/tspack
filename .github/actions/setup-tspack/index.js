const childProcess = require("child_process");
const crypto = require("crypto");
const fs = require("fs");
const http = require("http");
const https = require("https");
const os = require("os");
const path = require("path");

const DEFAULT_REPO = "yuechen-li-dev/tspack";

function getInput(name, defaultValue) {
  const key = `INPUT_${name.replace(/ /g, "_").replace(/-/g, "_").toUpperCase()}`;
  const value = process.env[key];

  if (value === undefined || value.trim() === "") {
    return defaultValue;
  }

  return value.trim();
}

function mapPlatform(platform, arch) {
  let osName;
  if (platform === "linux") {
    osName = "linux";
  } else if (platform === "darwin") {
    osName = "darwin";
  } else if (platform === "win32") {
    osName = "windows";
  } else {
    throw new Error(`Unsupported platform: ${platform}/${arch}`);
  }

  let archName;
  if (arch === "x64") {
    archName = "amd64";
  } else if (arch === "arm64") {
    archName = "arm64";
  } else {
    throw new Error(`Unsupported architecture: ${platform}/${arch}`);
  }

  if (osName === "windows" && archName !== "amd64") {
    throw new Error(`Unsupported platform: ${platform}/${arch}. Windows arm64 is not supported yet.`);
  }

  const extension = osName === "windows" ? ".zip" : ".tar.gz";
  const artifact = `tspack-${osName}-${archName}${extension}`;

  return {
    osName,
    archName,
    artifact,
    binaryName: osName === "windows" ? "tspack.exe" : "tspack",
  };
}

function normalizeBaseUrl(value, fallback) {
  const baseUrl = value || fallback;
  return baseUrl.replace(/\/+$/, "");
}

function buildDownloadUrls(repo, version, artifact) {
  const githubBase = normalizeBaseUrl(process.env.TSPACK_ACTION_GITHUB_BASE, "https://github.com");
  const releaseBase = `${githubBase}/${repo}/releases/download/${encodeURIComponent(version)}`;

  return {
    artifactUrl: `${releaseBase}/${artifact}`,
    checksumsUrl: `${releaseBase}/checksums.txt`,
  };
}

function parseLatestTagName(body) {
  let parsed;
  try {
    parsed = JSON.parse(body);
  } catch (error) {
    throw new Error(`GitHub latest release response was not valid JSON: ${error.message}`);
  }

  if (!parsed || typeof parsed.tag_name !== "string" || parsed.tag_name.trim() === "") {
    throw new Error("GitHub latest release response did not include tag_name.");
  }

  return parsed.tag_name.trim();
}

async function resolveVersion(versionInput, repo, token) {
  if (versionInput !== "latest") {
    return versionInput;
  }

  const apiBase = normalizeBaseUrl(process.env.TSPACK_ACTION_API_BASE, "https://api.github.com");
  const url = `${apiBase}/repos/${repo}/releases/latest`;
  const body = await requestText(url, token);
  return parseLatestTagName(body);
}

function parseChecksums(text) {
  const checksums = new Map();
  const lines = text.split(/\r?\n/);

  for (const line of lines) {
    const trimmed = line.trim();
    if (trimmed === "") {
      continue;
    }

    const match = trimmed.match(/^([a-fA-F0-9]{64})\s+\*?(.+)$/);
    if (!match) {
      continue;
    }

    checksums.set(path.basename(match[2].trim()), match[1].toLowerCase());
  }

  return checksums;
}

function sha256File(filePath) {
  const hash = crypto.createHash("sha256");
  const data = fs.readFileSync(filePath);
  hash.update(data);
  return hash.digest("hex");
}

function verifyChecksum(archivePath, checksumsText, artifact) {
  const checksums = parseChecksums(checksumsText);
  const expected = checksums.get(artifact);

  if (!expected) {
    throw new Error(`checksums.txt did not include ${artifact}.`);
  }

  const actual = sha256File(archivePath);
  if (actual !== expected) {
    throw new Error(`Checksum mismatch for ${artifact}: expected ${expected}, got ${actual}.`);
  }

  return actual;
}

function requestText(url, token) {
  return requestBuffer(url, token).then((body) => body.toString("utf8"));
}

function downloadFile(url, token, destination) {
  return requestBuffer(url, token).then((body) => {
    fs.writeFileSync(destination, body);
    return destination;
  });
}

function requestOptions(token) {
  const headers = {
    "User-Agent": "setup-tspack",
    Accept: "application/octet-stream, application/json, text/plain",
  };

  if (token) {
    headers.Authorization = `Bearer ${token}`;
  }

  return { headers };
}

function requestBuffer(url, token, redirectCount = 0) {
  if (redirectCount > 5) {
    return Promise.reject(new Error(`Too many redirects while requesting ${url}.`));
  }

  return new Promise((resolve, reject) => {
    const client = url.startsWith("http://") ? http : https;
    const request = client.get(url, requestOptions(token), (response) => {
      if (response.statusCode >= 300 && response.statusCode < 400 && response.headers.location) {
        const redirectUrl = new URL(response.headers.location, url).toString();
        response.resume();
        requestBuffer(redirectUrl, token, redirectCount + 1).then(resolve, reject);
        return;
      }

      const chunks = [];
      response.on("data", (chunk) => chunks.push(chunk));
      response.on("end", () => {
        const body = Buffer.concat(chunks);
        if (response.statusCode < 200 || response.statusCode >= 300) {
          reject(new Error(`Request failed with HTTP ${response.statusCode}: ${body.toString("utf8")}`));
          return;
        }

        resolve(body);
      });
    });

    request.on("error", reject);
  });
}

function runCommand(command, args) {
  const result = childProcess.spawnSync(command, args, {
    stdio: "inherit",
    windowsHide: true,
  });

  if (result.error) {
    throw new Error(`Failed to run ${command}: ${result.error.message}`);
  }

  if (result.status !== 0) {
    throw new Error(`${command} exited with status ${result.status}.`);
  }
}

function extractArchive(archivePath, extractDir, artifact) {
  if (artifact.endsWith(".tar.gz")) {
    runCommand("tar", ["-xzf", archivePath, "-C", extractDir]);
    return;
  }

  if (artifact.endsWith(".zip")) {
    const command = `Expand-Archive -Path ${quotePowerShell(archivePath)} -DestinationPath ${quotePowerShell(extractDir)} -Force`;
    runCommand("powershell", ["-NoProfile", "-Command", command]);
    return;
  }

  throw new Error(`Unsupported archive type: ${artifact}`);
}

function quotePowerShell(value) {
  return `'${value.replace(/'/g, "''")}'`;
}

function findExtractedBinary(rootDir, binaryName) {
  const queue = [rootDir];

  while (queue.length > 0) {
    const current = queue.shift();
    const entries = fs.readdirSync(current, { withFileTypes: true });

    for (const entry of entries) {
      const entryPath = path.join(current, entry.name);
      if (entry.isDirectory()) {
        queue.push(entryPath);
        continue;
      }

      if (entry.isFile() && entry.name === binaryName) {
        return entryPath;
      }
    }
  }

  throw new Error(`Could not find ${binaryName} in extracted archive.`);
}

function appendPath(installDir) {
  if (!process.env.GITHUB_PATH) {
    console.log(`Add ${installDir} to PATH for this shell.`);
    return;
  }

  fs.appendFileSync(process.env.GITHUB_PATH, `${installDir}${os.EOL}`);
}

function setOutput(name, value) {
  if (!process.env.GITHUB_OUTPUT) {
    console.log(`${name}=${value}`);
    return;
  }

  fs.appendFileSync(process.env.GITHUB_OUTPUT, `${name}=${value}${os.EOL}`);
}

function shouldRunCheck(value) {
  return !["false", "0", "no", "off"].includes(String(value).toLowerCase());
}

async function main() {
  const versionInput = getInput("version", "latest");
  const repo = getInput("repo", DEFAULT_REPO);
  const token = getInput("github-token", process.env.GITHUB_TOKEN || "");
  const installDir = getInput("install-dir", path.join(process.env.RUNNER_TEMP || os.tmpdir(), "tspack-bin"));
  const check = getInput("check", "true");
  const platformInfo = mapPlatform(process.platform, process.arch);

  const version = await resolveVersion(versionInput, repo, token);
  const urls = buildDownloadUrls(repo, version, platformInfo.artifact);
  const tempDir = fs.mkdtempSync(path.join(os.tmpdir(), "setup-tspack-"));
  const archivePath = path.join(tempDir, platformInfo.artifact);
  const extractDir = path.join(tempDir, "extract");

  fs.mkdirSync(extractDir, { recursive: true });
  fs.mkdirSync(installDir, { recursive: true });

  console.log(`Installing TSPack ${version} from ${repo}`);
  console.log(`Downloading ${platformInfo.artifact}`);
  await downloadFile(urls.artifactUrl, token, archivePath);

  console.log("Downloading checksums.txt");
  const checksumsText = await requestText(urls.checksumsUrl, token);
  verifyChecksum(archivePath, checksumsText, platformInfo.artifact);

  extractArchive(archivePath, extractDir, platformInfo.artifact);
  const extractedBinary = findExtractedBinary(extractDir, platformInfo.binaryName);
  const installedPath = path.join(installDir, platformInfo.binaryName);

  fs.copyFileSync(extractedBinary, installedPath);
  if (process.platform !== "win32") {
    fs.chmodSync(installedPath, 0o755);
  }

  appendPath(installDir);
  setOutput("version", version);
  setOutput("path", installedPath);

  if (shouldRunCheck(check)) {
    runCommand(installedPath, ["--help"]);
  }

  console.log(`TSPack ${version} installed at ${installedPath}`);
}

if (require.main === module) {
  main().catch((error) => {
    console.error(error.message);
    process.exit(1);
  });
}

module.exports = {
  buildDownloadUrls,
  findExtractedBinary,
  mapPlatform,
  parseChecksums,
  parseLatestTagName,
  resolveVersion,
  shouldRunCheck,
  verifyChecksum,
};
