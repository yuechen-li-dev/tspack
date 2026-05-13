#!/usr/bin/env node
import fs from 'node:fs';
import os from 'node:os';
import path from 'node:path';
import net from 'node:net';
import { spawn } from 'node:child_process';

const candidates = ['code', 'code-insiders', 'code-oss', 'codium', 'vscodium'];

function resolveVSCodeElectronExecutable(wrapperPath) {
  const wrapperName = path.basename(wrapperPath);
  const variants = {
    code: ['/usr/share/code/code'],
    'code-insiders': ['/usr/share/code-insiders/code-insiders'],
    codium: ['/usr/share/codium/codium'],
    'code-oss': ['/usr/share/code-oss/code-oss'],
    vscodium: ['/usr/share/vscodium/vscodium']
  };

  const wrapperDirCandidate = path.resolve(path.dirname(wrapperPath), '..', 'share', wrapperName, wrapperName);
  const knownCandidates = variants[wrapperName] ?? [];
  const allCandidates = [wrapperDirCandidate, ...knownCandidates, wrapperPath];
  for (const candidate of allCandidates) {
    if (fs.existsSync(candidate) && fs.statSync(candidate).isFile()) {
      return candidate;
    }
  }
  return wrapperPath;
}

function tailText(input) {
  return input.trim().split('\n').slice(-8).join(' | ');
}

function freePort() {
  return new Promise((resolve, reject) => {
    const server = net.createServer();
    server.listen(0, '127.0.0.1', () => {
      const address = server.address();
      if (!address || typeof address === 'string') return reject(new Error('PORT_ALLOC_FAILED'));
      server.close((error) => (error ? reject(error) : resolve(address.port)));
    });
  });
}

async function runVersion(executable) {
  return new Promise((resolve) => {
    const child = spawn(executable, ['--version'], { stdio: ['ignore', 'pipe', 'pipe'] });
    let out = '';
    let err = '';
    child.stdout.on('data', (chunk) => { out += chunk.toString(); });
    child.stderr.on('data', (chunk) => { err += chunk.toString(); });
    child.on('exit', (code) => resolve({ code, out: out.trim(), err: err.trim() }));
    setTimeout(() => child.kill('SIGTERM'), 3000);
  });
}

async function probeMode(executablePath, modeName, extraArgs) {
  const port = await freePort();
  const userDataDir = fs.mkdtempSync(path.join(os.tmpdir(), `tspack-probe-${modeName}-`));
  const workspaceDir = fs.mkdtempSync(path.join(os.tmpdir(), 'tspack-probe-workspace-'));
  const endpoint = `http://127.0.0.1:${port}`;
  const args = [
    ...extraArgs,
    `--remote-debugging-port=${port}`,
    `--user-data-dir=${userDataDir}`,
    '--no-first-run',
    '--no-default-browser-check',
    '--disable-background-networking',
    '--no-sandbox',
    '--disable-gpu',
    '--disable-dev-shm-usage',
    '--new-window'
  ];

  const resolvedArgs = args.map((item) => (item === '__WORKSPACE__' ? workspaceDir : item));
  const child = spawn(executablePath, resolvedArgs, { stdio: ['ignore', 'pipe', 'pipe'] });
  let stderr = '';
  let stdout = '';
  child.stdout.on('data', (chunk) => { stdout += chunk.toString(); });
  child.stderr.on('data', (chunk) => { stderr += chunk.toString(); });

  const result = { modeName, args: resolvedArgs, endpoint, versionReady: false, targets: [] };
  for (let i = 0; i < 80; i += 1) {
    if (child.exitCode !== null) {
      break;
    }
    try {
      const versionResponse = await fetch(`${endpoint}/json/version`);
      if (versionResponse.ok) {
        result.versionReady = true;
        const targetResponse = await fetch(`${endpoint}/json/list`);
        if (targetResponse.ok) {
          result.targets = await targetResponse.json();
        }
        break;
      }
    } catch {
      // poll
    }
    await new Promise((resolve) => setTimeout(resolve, 100));
  }

  if (child.exitCode === null) {
    child.kill('SIGTERM');
    await new Promise((resolve) => {
      child.once('exit', () => resolve());
      setTimeout(() => resolve(), 1500);
    });
  }

  fs.rmSync(userDataDir, { recursive: true, force: true });
  fs.rmSync(workspaceDir, { recursive: true, force: true });

  return {
    ...result,
    exitCode: child.exitCode,
    signal: child.signalCode,
    stderrTail: tailText(stderr),
    stdoutTail: tailText(stdout)
  };
}

async function main() {
  console.log(`OS: ${os.platform()} ${os.release()}`);
  console.log(`Arch: ${os.arch()}`);
  if (fs.existsSync('/etc/os-release')) {
    console.log('Distro:');
    console.log(fs.readFileSync('/etc/os-release', 'utf8').trim());
  }

  const found = [];
  for (const name of candidates) {
    const pathParts = (process.env.PATH ?? '').split(':').filter(Boolean);
    const fullPath = pathParts.map((part) => `${part}/${name}`).find((candidatePath) => fs.existsSync(candidatePath)) || '';
    if (!fullPath) {
      console.log(`${name}: not found`);
      continue;
    }
    const version = await runVersion(fullPath);
    console.log(`${name}: ${fullPath}`);
    console.log(`  --version exit=${version.code}`);
    if (version.out) console.log(`  stdout: ${version.out}`);
    if (version.err) console.log(`  stderr: ${version.err}`);
    found.push(fullPath);
  }

  if (found.length === 0) {
    console.log('No Code-family executable found.');
    console.log('Next steps:');
    console.log('- Install Code - OSS / VSCodium / VS Code in this environment.');
    console.log('- Debian/Ubuntu note: branded `code` usually comes from Microsoft repo; `codium` usually comes from VSCodium distribution channels.');
    return;
  }

  const wrapperPath = process.env.TSPACK_INSPECT_HOST_PATH || found[0];
  const executablePath = resolveVSCodeElectronExecutable(wrapperPath);
  console.log(`\nInput path: ${wrapperPath}`);
  console.log(`Resolved executable path: ${executablePath}`);
  console.log(`Launch mode: explicit host path`);
  const modes = [
    { name: 'workbench', extraArgs: [] },
    { name: 'workspace', extraArgs: ['__WORKSPACE__'] },
    { name: 'url', extraArgs: ['http://127.0.0.1:5173'] }
  ];

  let anyReady = false;
  let anyAttempt = false;
  for (const mode of modes) {
    const result = await probeMode(executablePath, mode.name, mode.extraArgs);
    anyAttempt = true;
    if (result.versionReady) anyReady = true;
    console.log(`\nMode: ${result.modeName}`);
    console.log(`  args: ${result.args.join(' ')}`);
    console.log(`  /json/version ready: ${result.versionReady}`);
    console.log(`  exit: code=${result.exitCode} signal=${result.signal}`);
    if (result.versionReady) {
      console.log(`  /json/list targets: ${result.targets.length}`);
      console.log(JSON.stringify(result.targets.slice(0, 3), null, 2));
    }
    if (result.stderrTail) console.log(`  stderr tail: ${result.stderrTail}`);
    if (result.stdoutTail) console.log(`  stdout tail: ${result.stdoutTail}`);
  }

  if (!anyAttempt) {
    console.log('outcome: unavailable');
    return;
  }

  console.log(`\noutcome: ${anyReady ? 'usable' : 'not-usable'}`);
}

main().catch((error) => {
  console.error(error);
  process.exit(1);
});
