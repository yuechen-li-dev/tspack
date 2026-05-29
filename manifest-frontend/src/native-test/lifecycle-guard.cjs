const Module = require('node:module');
const originalFs = require('node:fs');
const originalPath = require('node:path');
const originalUrl = require('node:url');
const unguardedWriteFileSync = originalFs.writeFileSync.bind(originalFs);

const reportPath = process.env.TSPACK_LIFECYCLE_GUARD_REPORT;
const configPath = process.env.TSPACK_LIFECYCLE_GUARD_CONFIG;
const state = {
  violations: [],
  reads: [],
  writes: [],
  guardFailures: [],
};

let config = {
  packageDir: process.cwd(),
  tempDir:
    process.env.TMPDIR || process.env.TEMP || process.env.TMP || process.cwd(),
  denyNetwork: true,
  denyChildProcess: true,
  denyEnv: [],
  allowReadRoots: [process.cwd()],
  allowWriteRoots: [process.cwd()],
};

try {
  if (configPath) {
    const rawConfig = originalFs.readFileSync(configPath, 'utf8');
    config = Object.assign(config, JSON.parse(rawConfig));
  }
} catch (error) {
  state.guardFailures.push(`failed to read guard config: ${error.message}`);
}

const deniedEnvKeys = new Set(
  (config.denyEnv || []).map((key) => String(key).toUpperCase()),
);
const allowReadRoots = normalizeRoots(config.allowReadRoots || []);
const allowWriteRoots = normalizeRoots(config.allowWriteRoots || []);
const originalEnv = process.env;
const originalLoad = Module._load;

function normalizeRoots(roots) {
  return roots.map((root) => originalPath.resolve(String(root)));
}

function recordViolation(violation) {
  state.violations.push(violation);
}

function recordUnique(list, value) {
  if (!list.includes(value)) {
    list.push(value);
  }
}

function createDeniedError(code, detail) {
  const error = new Error(`${code}: ${detail}`);
  error.code = code;
  return error;
}

function resolveProbePath(targetPath) {
  if (targetPath instanceof URL) {
    return originalPath.resolve(originalUrl.fileURLToPath(targetPath));
  }
  if (typeof targetPath !== 'string') {
    return undefined;
  }
  if (targetPath.startsWith('file://')) {
    try {
      return originalPath.resolve(originalUrl.fileURLToPath(targetPath));
    } catch (_error) {
      return originalPath.resolve(String(targetPath));
    }
  }
  return originalPath.resolve(process.cwd(), targetPath);
}

function isInsideAnyRoot(filePath, roots) {
  const resolved = originalPath.resolve(filePath);
  return roots.some((root) => {
    const relative = originalPath.relative(root, resolved);
    return (
      relative === '' ||
      (!relative.startsWith('..') && !originalPath.isAbsolute(relative))
    );
  });
}

function checkRead(targetPath, operation) {
  const resolved = resolveProbePath(targetPath);
  if (!resolved) {
    return;
  }
  recordUnique(state.reads, resolved);
  if (!isInsideAnyRoot(resolved, allowReadRoots)) {
    recordViolation({
      code: 'TSPACK_LIFECYCLE_FS_READ_DENIED',
      kind: 'fs',
      detail: operation,
      path: resolved,
    });
    throw createDeniedError('TSPACK_LIFECYCLE_FS_READ_DENIED', resolved);
  }
}

function checkWrite(targetPath, operation) {
  const resolved = resolveProbePath(targetPath);
  if (!resolved) {
    return;
  }
  recordUnique(state.writes, resolved);
  if (!isInsideAnyRoot(resolved, allowWriteRoots)) {
    recordViolation({
      code: 'TSPACK_LIFECYCLE_FS_WRITE_DENIED',
      kind: 'fs',
      detail: operation,
      path: resolved,
    });
    throw createDeniedError('TSPACK_LIFECYCLE_FS_WRITE_DENIED', resolved);
  }
}

function denyNetwork(moduleName, operation) {
  recordViolation({
    code: 'TSPACK_LIFECYCLE_NETWORK_DENIED',
    kind: 'network',
    detail: operation,
    module: moduleName,
  });
  throw createDeniedError(
    'TSPACK_LIFECYCLE_NETWORK_DENIED',
    `${moduleName}.${operation}`,
  );
}

function denyChildProcess(operation) {
  recordViolation({
    code: 'TSPACK_LIFECYCLE_CHILD_PROCESS_DENIED',
    kind: 'child_process',
    detail: operation,
    module: 'child_process',
  });
  throw createDeniedError(
    'TSPACK_LIFECYCLE_CHILD_PROCESS_DENIED',
    `child_process.${operation}`,
  );
}

function patchNetworkModule(moduleName, moduleExports) {
  if (!config.denyNetwork) {
    return moduleExports;
  }
  for (const operation of ['request', 'get']) {
    if (typeof moduleExports[operation] === 'function') {
      moduleExports[operation] = function deniedNetworkCall() {
        return denyNetwork(moduleName, operation);
      };
    }
  }
  return moduleExports;
}

function patchNetModule(moduleName, moduleExports) {
  if (!config.denyNetwork) {
    return moduleExports;
  }
  for (const operation of ['connect', 'createConnection']) {
    if (typeof moduleExports[operation] === 'function') {
      moduleExports[operation] = function deniedNetworkCall() {
        return denyNetwork(moduleName, operation);
      };
    }
  }
  return moduleExports;
}

function patchDnsModule(moduleExports) {
  if (!config.denyNetwork) {
    return moduleExports;
  }
  for (const operation of [
    'lookup',
    'resolve',
    'resolve4',
    'resolve6',
    'resolveMx',
    'resolveTxt',
  ]) {
    if (typeof moduleExports[operation] === 'function') {
      moduleExports[operation] = function deniedDnsCall() {
        return denyNetwork('dns', operation);
      };
    }
  }
  return moduleExports;
}

function patchChildProcessModule(moduleExports) {
  if (!config.denyChildProcess) {
    return moduleExports;
  }
  for (const operation of ['spawn', 'exec', 'execFile', 'fork']) {
    if (typeof moduleExports[operation] === 'function') {
      moduleExports[operation] = function deniedChildProcessCall() {
        return denyChildProcess(operation);
      };
    }
  }
  return moduleExports;
}

function patchFsModule(moduleExports) {
  const readOperations = ['readFile', 'readFileSync', 'createReadStream'];
  const writeOperations = [
    'writeFile',
    'writeFileSync',
    'appendFile',
    'appendFileSync',
    'mkdir',
    'mkdirSync',
    'rm',
    'rmSync',
    'unlink',
    'unlinkSync',
    'rename',
    'renameSync',
    'copyFile',
    'copyFileSync',
    'createWriteStream',
  ];

  for (const operation of readOperations) {
    if (typeof moduleExports[operation] === 'function') {
      const original = moduleExports[operation];
      moduleExports[operation] = function guardedRead(targetPath, ...args) {
        checkRead(targetPath, operation);
        return original.call(this, targetPath, ...args);
      };
    }
  }

  for (const operation of writeOperations) {
    if (typeof moduleExports[operation] === 'function') {
      const original = moduleExports[operation];
      moduleExports[operation] = function guardedWrite(targetPath, ...args) {
        checkWrite(targetPath, operation);
        if (
          operation === 'rename' ||
          operation === 'renameSync' ||
          operation === 'copyFile' ||
          operation === 'copyFileSync'
        ) {
          checkWrite(args[0], operation);
        }
        return original.call(this, targetPath, ...args);
      };
    }
  }

  return moduleExports;
}

function patchLoadedModule(request, moduleExports) {
  if (request === 'http' || request === 'node:http') {
    return patchNetworkModule('http', moduleExports);
  }
  if (request === 'https' || request === 'node:https') {
    return patchNetworkModule('https', moduleExports);
  }
  if (request === 'net' || request === 'node:net') {
    return patchNetModule('net', moduleExports);
  }
  if (request === 'tls' || request === 'node:tls') {
    return patchNetModule('tls', moduleExports);
  }
  if (request === 'dns' || request === 'node:dns') {
    return patchDnsModule(moduleExports);
  }
  if (request === 'child_process' || request === 'node:child_process') {
    return patchChildProcessModule(moduleExports);
  }
  if (request === 'fs' || request === 'node:fs') {
    return patchFsModule(moduleExports);
  }
  return moduleExports;
}

function installEnvProxy() {
  const proxy = new Proxy(originalEnv, {
    get(target, property, receiver) {
      if (
        typeof property === 'string' &&
        deniedEnvKeys.has(property.toUpperCase())
      ) {
        recordViolation({
          code: 'TSPACK_LIFECYCLE_ENV_DENIED',
          kind: 'env',
          detail: `read process.env.${property}`,
          envKey: property,
        });
        return undefined;
      }
      return Reflect.get(target, property, receiver);
    },
    has(target, property) {
      if (
        typeof property === 'string' &&
        deniedEnvKeys.has(property.toUpperCase())
      ) {
        recordViolation({
          code: 'TSPACK_LIFECYCLE_ENV_DENIED',
          kind: 'env',
          detail: `checked process.env.${property}`,
          envKey: property,
        });
        return false;
      }
      return Reflect.has(target, property);
    },
  });

  try {
    Object.defineProperty(process, 'env', {
      configurable: true,
      enumerable: true,
      value: proxy,
      writable: true,
    });
  } catch (error) {
    state.guardFailures.push(`failed to install env proxy: ${error.message}`);
  }
}

Module._load = function guardedModuleLoad(request, parent, isMain) {
  const moduleExports = originalLoad.call(this, request, parent, isMain);
  return patchLoadedModule(request, moduleExports);
};

installEnvProxy();
patchFsModule(originalFs);

process.on('exit', () => {
  if (!reportPath) {
    return;
  }

  try {
    const report = JSON.stringify({
      violations: state.violations,
      reads: state.reads,
      writes: state.writes,
      guardFailures: state.guardFailures,
    }, null, 2);
    unguardedWriteFileSync(reportPath, report, 'utf8');
  } catch (_error) {
    // No stdout/stderr marker fallback: the harness deliberately treats a missing
    // report file as TSPACK_LIFECYCLE_REPORT_MISSING.
  }
});
