import { execFileSync, spawnSync } from "node:child_process";
import { mkdirSync, readFileSync, readdirSync, statSync, writeFileSync } from "node:fs";
import { arch, cpus, platform, release, totalmem } from "node:os";
import { dirname, resolve } from "node:path";
import { fileURLToPath } from "node:url";

const benchmarkRoot = dirname(fileURLToPath(import.meta.url));
const repositoryRoot = resolve(benchmarkRoot, "../..");
const copelandRoot = resolve(repositoryRoot, "../Copeland");
const perryFixtureRoot = resolve(repositoryRoot, "fixtures/perry-hotpath-m71b");
const perrySourcePath = resolve(perryFixtureRoot, "src/hot/bench.ts");
const perryCommand = platform() === "win32"
  ? resolve(perryFixtureRoot, "node_modules/@perryts/perry-win32-x64/bin/perry.exe")
  : resolve(perryFixtureRoot, "node_modules/.bin/perry");
const perryExecutable = resolve(perryFixtureRoot, "dist/hotpath-m71d.exe");
const copelandOutput = resolve(copelandRoot, "artifacts/m71d-qualification");
const artifactPath = resolve(repositoryRoot, "artifacts/m71d/perf-results.json");
const publishedCrosscheckRoot = resolve(repositoryRoot, "artifacts/m71d/perry-published-crosscheck");

const workloadNames = [
  "integer-loop-m71a",
  "floating-point",
  "image-convolution-5x5",
  "primitive-array-rw",
  "array-of-records",
  "allocation-churn",
  "byte-hash",
  "json-roundtrip",
  "branch-heavy",
  "function-calls",
];

function run(executable, args, cwd, options = {}) {
  const started = performance.now();
  const completed = spawnSync(executable, args, {
    cwd,
    encoding: "utf8",
    maxBuffer: 32 * 1024 * 1024,
    env: options.env ?? process.env,
  });
  const elapsedMs = performance.now() - started;
  if (completed.status !== 0) {
    throw new Error(`${executable} failed (${completed.status}):\n${completed.stdout}\n${completed.stderr}`);
  }
  return { stdout: completed.stdout.trim(), stderr: completed.stderr.trim(), elapsedMs };
}

function runJson(executable, args, cwd, options) {
  return JSON.parse(run(executable, args, cwd, options).stdout);
}

function percentile(sorted, fraction) {
  const index = Math.min(sorted.length - 1, Math.max(0, Math.ceil(sorted.length * fraction) - 1));
  return sorted[index];
}

function statistics(samples) {
  const sorted = [...samples].sort((left, right) => left - right);
  const medianMs = percentile(sorted, 0.5);
  const deviations = sorted.map((value) => Math.abs(value - medianMs)).sort((left, right) => left - right);
  const mean = sorted.reduce((sum, value) => sum + value, 0) / sorted.length;
  const variance = sorted.reduce((sum, value) => sum + ((value - mean) ** 2), 0) / sorted.length;
  return {
    minMs: sorted[0],
    p05Ms: percentile(sorted, 0.05),
    medianMs,
    p95Ms: percentile(sorted, 0.95),
    maxMs: sorted[sorted.length - 1],
    madMs: percentile(deviations, 0.5),
    stddevMs: Math.sqrt(variance),
  };
}

function summarize(implementation, report) {
  return report.workloads.map((workload) => ({
    workload: workload.name,
    implementation,
    warmup: workload.warmup,
    sampleCount: workload.correct ? workload.samplesMs.length : 0,
    checksum: workload.checksum,
    correct: workload.correct,
    statistics: workload.correct ? statistics(workload.samplesMs) : null,
    samplesMs: workload.correct ? workload.samplesMs : [],
  }));
}

function directorySize(path) {
  let bytes = 0;
  for (const entry of readdirSync(path, { withFileTypes: true })) {
    const entryPath = resolve(path, entry.name);
    bytes += entry.isDirectory() ? directorySize(entryPath) : statSync(entryPath).size;
  }
  return bytes;
}

function coldSamples(executable, args, cwd, count, options) {
  const samples = [];
  for (let index = 0; index < count; index += 1) {
    samples.push(run(executable, args, cwd, options).elapsedMs);
  }
  return { samplesMs: samples, statistics: statistics(samples) };
}

function publishedSamples(executable, args, cwd, label, checksumLabel, count, options) {
  const samples = [];
  let checksum = null;
  for (let index = 0; index < count; index += 1) {
    const lines = run(executable, args, cwd, options).stdout.split(/\r?\n/);
    const elapsedLine = lines.find((line) => line.startsWith(`${label}:`));
    const checksumLine = lines.find((line) => line.startsWith(`${checksumLabel}:`));
    if (!elapsedLine || !checksumLine) {
      throw new Error(`Published benchmark ${label} returned an unexpected envelope: ${lines.join(" | ")}`);
    }
    samples.push(Number(elapsedLine.slice(label.length + 1)));
    checksum = Number(checksumLine.slice(checksumLabel.length + 1));
  }
  return { samplesMs: samples, statistics: statistics(samples), checksum };
}

function gitCommit(path) {
  return execFileSync("git", ["-C", path, "rev-parse", "HEAD"], { encoding: "utf8" }).trim();
}

function executableName() {
  return platform() === "win32" ? "CtsJitM0Host.exe" : "CtsJitM0Host";
}

function camelize(value) {
  if (Array.isArray(value)) {
    return value.map(camelize);
  }
  if (value === null || typeof value !== "object") {
    return value;
  }
  return Object.fromEntries(Object.entries(value).map(([key, entry]) => [
    key[0].toLowerCase() + key.slice(1),
    camelize(entry),
  ]));
}

// Keep compiler acquisition and target provenance on TSPack's real project
// path. The lab then invokes the materialized Perry binary directly so its
// internal kernel clock is not confused with a TSPack/sidecar boundary clock.
run("go", ["run", "./cmd/tspack", "build", "hotpath", "--root", perryFixtureRoot], repositoryRoot);

const copelandStarted = performance.now();
run(
  "dotnet",
  [
    "run",
    "--project",
    "tools/CtsJitM0/CtsJitM0.csproj",
    "--configuration",
    "Release",
    "--",
    "--output",
    "artifacts/m71d-qualification",
    "--cold-runs",
    "10",
    "--warmup-rounds",
    "10",
    "--measured-rounds",
    "20",
    "--javascript-profile",
    "production",
  ],
  copelandRoot,
);
const copelandBuildAndRunMs = performance.now() - copelandStarted;
const copeland = camelize(JSON.parse(readFileSync(resolve(copelandOutput, "results.json"), "utf8")));

const nativeRows = [];
const nativeBuilds = [];
for (const workload of copeland.workloads) {
  const hostRoot = resolve(copelandOutput, "generated", workload.name, "csharp-host");
  const publishRoot = resolve(hostRoot, "publish");
  const publication = run(
    "dotnet",
    [
      "publish",
      "CtsJitM0Host.csproj",
      "--configuration",
      "Release",
      "--runtime",
      "win-x64",
      "--property:PublishAot=true",
      "--property:SelfContained=true",
      "--property:InvariantGlobalization=true",
      "--property:DebugType=None",
      "--property:StripSymbols=true",
      "--output",
      "publish",
    ],
    hostRoot,
  );
  const executable = resolve(publishRoot, executableName());
  const checksum = runJson(executable, ["--checksum"], hostRoot).checksum;
  const measured = runJson(
    executable,
    ["--warm", "--warmup", "10", "--rounds", "20", "--iterations", String(workload.iterationsPerRound)],
    hostRoot,
  );
  const correct = checksum === runJson("dotnet", [resolve(hostRoot, "bin/Release/net10.0/CtsJitM0Host.dll"), "--checksum"], hostRoot).checksum
    && measured.checksum === workload.checksum;
  nativeRows.push({
    workload: workload.name,
    implementation: "Copeland/NativeAOT",
    warmup: 10,
    sampleCount: correct ? measured.milliseconds.length : 0,
    checksum: measured.checksum,
    correct,
    statistics: correct ? statistics(measured.milliseconds) : null,
    samplesMs: correct ? measured.milliseconds : [],
    allocatedBytes: measured.allocatedBytes,
    gcCollections: measured.gcCollections,
  });
  nativeBuilds.push({
    workload: workload.name,
    publishMs: publication.elapsedMs,
    executableBytes: statSync(executable).size,
    publishFootprintBytes: directorySize(publishRoot),
    executable,
  });
}

const doctor = run(perryCommand, ["doctor"], perryFixtureRoot).stdout;
const runtimeMatch = doctor.match(/runtime library:\s*(.+perry_runtime\.lib)/i);
if (!runtimeMatch) {
  throw new Error("Perry doctor did not report its runtime library path.");
}
const perryEnvironment = { ...process.env, PERRY_RUNTIME_DIR: dirname(runtimeMatch[1].trim()) };
const perryCompileArgs = [
  "compile",
  "src/hot/bench.ts",
  "-o",
  "dist/hotpath-m71d.exe",
  "--target",
  "windows",
  "--no-codegen",
  "--fp-contract",
  "off",
];
const perryCleanCompile = run(perryCommand, [...perryCompileArgs, "--no-cache"], perryFixtureRoot, { env: perryEnvironment });
const perryWarmCompile = run(perryCommand, perryCompileArgs, perryFixtureRoot, { env: perryEnvironment });

const publishedCrosscheck = [];
for (const definition of [
  { name: "array_read", file: "array-read.ts", checksumLabel: "sum" },
  { name: "loop_overhead", file: "loop-overhead.ts", checksumLabel: "sum" },
]) {
  const executable = resolve(publishedCrosscheckRoot, `${definition.name}.exe`);
  run(
    perryCommand,
    ["compile", definition.file, "-o", executable, "--target", "windows", "--no-codegen", "--fp-contract", "off", "--no-cache"],
    publishedCrosscheckRoot,
    { env: perryEnvironment },
  );
  const node = publishedSamples(process.execPath, [resolve(publishedCrosscheckRoot, definition.file)], publishedCrosscheckRoot, definition.name, definition.checksumLabel, 11);
  const perry = publishedSamples(executable, [], publishedCrosscheckRoot, definition.name, definition.checksumLabel, 11);
  publishedCrosscheck.push({
    workload: definition.name,
    source: definition.file,
    node,
    perry,
    checksumMatch: node.checksum === perry.checksum,
  });
}

const nodeRows = [];
const perryRows = [];
for (const workloadName of workloadNames) {
  nodeRows.push(...summarize("Node/V8", runJson(process.execPath, [perrySourcePath, "--workload", workloadName], perryFixtureRoot)));
  perryRows.push(...summarize("Perry", runJson(perryExecutable, ["--workload", workloadName], perryFixtureRoot)));
}
const nodeByName = new Map(nodeRows.map((row) => [row.workload, row]));
const perryCorrectness = [];
for (const row of perryRows) {
  const nodeRow = nodeByName.get(row.workload);
  const match = row.correct && nodeRow?.checksum === row.checksum;
  perryCorrectness.push({ workload: row.workload, match, nodeChecksum: nodeRow?.checksum, perryChecksum: row.checksum });
  if (!match) {
    row.correct = false;
    row.sampleCount = 0;
    row.statistics = null;
    row.samplesMs = [];
  }
}

const copelandRows = [];
for (const workload of copeland.workloads) {
  copelandRows.push({
    workload: workload.name,
    implementation: "Copeland/V8",
    warmup: copeland.protocol.warmupRounds,
    sampleCount: workload.javaScriptWarm.milliseconds.length,
    checksum: workload.javaScriptWarm.checksum,
    correct: workload.javaScriptWarm.checksum === workload.checksum,
    statistics: statistics(workload.javaScriptWarm.milliseconds),
    samplesMs: workload.javaScriptWarm.milliseconds,
    heapDeltaBytes: workload.javaScriptWarm.heapDeltaBytes,
  });
  copelandRows.push({
    workload: workload.name,
    implementation: "Copeland/RyuJIT",
    warmup: copeland.protocol.warmupRounds,
    sampleCount: workload.cSharpWarm.milliseconds.length,
    checksum: workload.cSharpWarm.checksum,
    correct: workload.cSharpWarm.checksum === workload.checksum,
    statistics: statistics(workload.cSharpWarm.milliseconds),
    samplesMs: workload.cSharpWarm.milliseconds,
    allocatedBytes: workload.cSharpWarm.allocatedBytes,
    gcCollections: workload.cSharpWarm.gcCollections,
  });
}

const representativeHost = resolve(copelandOutput, "generated/numeric-kernel/csharp-host");
const representativeManaged = resolve(representativeHost, "bin/Release/net10.0/CtsJitM0Host.dll");
const representativeNative = resolve(representativeHost, "publish", executableName());
const coldStart = {
  node: coldSamples(process.execPath, [perrySourcePath, "--probe", ""], perryFixtureRoot, 10),
  ryuJit: coldSamples("dotnet", [representativeManaged, "--checksum"], representativeHost, 10),
  nativeAot: coldSamples(representativeNative, ["--checksum"], representativeHost, 10),
  perry: coldSamples(perryExecutable, ["--probe", ""], perryFixtureRoot, 10),
  bun: null,
};

const result = {
  schemaVersion: 1,
  timestamp: new Date().toISOString(),
  host: {
    os: `${platform()} ${release()}`,
    architecture: arch(),
    cpu: cpus()[0]?.model ?? "unknown",
    logicalProcessors: cpus().length,
    ramBytes: totalmem(),
    powerMode: "Balanced",
  },
  tools: {
    node: process.version,
    v8: process.versions.v8,
    bun: null,
    dotnetSdk: run("dotnet", ["--version"], repositoryRoot).stdout,
    dotnetRuntime: copeland.environment.dotnetInfo,
    perry: run(perryCommand, ["--version"], perryFixtureRoot).stdout,
    llvm: run("clang", ["--version"], repositoryRoot).stdout.split(/\r?\n/)[0],
  },
  commits: { tspack: gitCommit(repositoryRoot), copeland: gitCommit(copelandRoot) },
  methodology: {
    independentSuite: "one process per selected workload; five warmups and eleven internal samples",
    copelandSuite: "same post-static MIR emitted to production JS and C#, ten warmups and twenty internal samples",
    nativeAot: "the exact generated C# host used by RyuJIT, published Release win-x64 with PublishAot and internal timing",
    correctness: "performance samples are removed whenever the lane checksum differs from its reference",
    tiering: copeland.environment.ryuJitConfiguration,
    v8: copeland.environment.v8Configuration,
  },
  benchmarkDefinitions: {
    independent: workloadNames,
    copeland: copeland.workloads.map((workload) => ({
      name: workload.name,
      sourceFile: workload.sourceFile,
      iterationsPerRound: workload.iterationsPerRound,
      checksum: workload.checksum,
    })),
    unsupportedCopelandSemanticFamilies: ["mutable typed-array write", "runtime JSON parse/stringify"],
  },
  correctness: { perry: perryCorrectness, copelandBackendParity: nativeRows.map((row) => ({ workload: row.workload, match: row.correct })) },
  publishedCrosscheck,
  workloads: [...nodeRows, ...perryRows, ...copelandRows, ...nativeRows],
  coldStart,
  compileBuild: {
    copelandBuildAndRunMs,
    perryCleanMs: perryCleanCompile.elapsedMs,
    perryIncrementalMs: perryWarmCompile.elapsedMs,
    nativeAot: nativeBuilds.map(({ workload, publishMs }) => ({ workload, publishMs })),
  },
  artifacts: {
    independentSourceBytes: statSync(perrySourcePath).size,
    perryExecutableBytes: statSync(perryExecutable).size,
    nativeAot: nativeBuilds.map(({ workload, executableBytes, publishFootprintBytes }) => ({ workload, executableBytes, publishFootprintBytes })),
    copeland: copeland.workloads.map((workload) => ({ workload: workload.name, ...workload.artifactSizes })),
  },
  rss: {
    status: "not measured by the primary harness because process polling perturbs short kernels; retained as a documented deviation",
  },
  runtimeMetadata: {
    ryuJit: { configuration: copeland.environment.ryuJitConfiguration, dynamicPgo: "production default; COMPlus overrides unset", readyToRun: false },
    nativeAot: { rid: "win-x64", publishAot: true, selfContained: true, invariantGlobalization: true, debugType: "None", stripSymbols: true },
    perry: { flags: ["--target", "windows", "--no-codegen", "--fp-contract", "off"], fastMath: false, currentReleaseVerified: "0.5.1220" },
  },
};

mkdirSync(dirname(artifactPath), { recursive: true });
writeFileSync(artifactPath, `${JSON.stringify(result, null, 2)}\n`);
console.log(artifactPath);
