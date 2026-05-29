import fs from "node:fs";
import path from "node:path";
import {
  createNativeArtifactReport,
  createNativeBenchmarkReport,
  createNativeDoomReport,
  createNativeTestReport,
  discoverNativeTestFile,
  formatNativeArtifactJsonReport,
  formatNativeArtifactTextReport,
  formatNativeBenchmarkJsonReport,
  formatNativeBenchmarkTextReport,
  formatNativeDoomJsonReport,
  formatNativeDoomTextReport,
  formatNativeTestJsonReport,
  formatNativeTestTextReport,
  formatNativeTestCompactTextReport,
  listNativeArtifacts,
  listNativeBenchmarks,
  listNativeProphecies,
  listNativeTests,
  nativeArtifactExitCode,
  nativeBenchmarkExitCode,
  nativeDoomExitCode,
  nativeTestExitCode,
  runNativeArtifacts,
  runNativeBenchmarks,
  runNativeProphecies,
  runNativeTestFiles,
} from "./native-test/index.js";

type Mode = "test" | "artifact" | "bench" | "doom" | "doom-child";
type Options = {
  mode: Mode;
  rootDir: string;
  list: boolean;
  filter?: string;
  json: boolean;
  compact: boolean;
  updateSnapshots: boolean;
  batch: boolean;
  files: string[];
  out?: string;
};

function parseArgs(argv: string[]): Options {
  const mode = (
    argv[2] === "artifact"
      ? "artifact"
      : argv[2] === "bench"
        ? "bench"
        : argv[2] === "doom"
          ? "doom"
          : argv[2] === "doom-child"
            ? "doom-child"
            : "test"
  ) as Mode;
  const options: Options = {
    mode,
    rootDir: ".",
    list: false,
    json: false,
    compact: false,
    updateSnapshots: false,
    batch: false,
    files: [],
  };
  for (let i = 3; i < argv.length; i += 1) {
    const arg = argv[i];
    if (arg === "--root") {
      i += 1;
      options.rootDir = argv[i] ?? ".";
      continue;
    }
    if (arg === "--list") {
      options.list = true;
      continue;
    }
    if (arg === "--filter") {
      i += 1;
      options.filter = argv[i];
      continue;
    }
    if (arg === "--json") {
      options.json = true;
      continue;
    }
    if (arg === "--compact") {
      options.compact = true;
      continue;
    }
    if (arg === "--update-snapshots") {
      options.updateSnapshots = true;
      continue;
    }
    if (arg === "--batch") {
      options.batch = true;
      continue;
    }
    if (arg === "--out") {
      i += 1;
      options.out = argv[i];
      continue;
    }
    if (arg === "--file") {
      i += 1;
      const file = argv[i];
      if (file) {
        options.files.push(file);
      }
      continue;
    }
    if (arg === "--id") {
      i += 1;
      continue;
    }
    throw new Error(`unknown flag: ${arg}`);
  }
  return options;
}

async function main(): Promise<void> {
  const options = parseArgs(process.argv);
  if (options.mode === "doom-child") {
    const file = process.argv[process.argv.indexOf("--file") + 1];
    const id = process.argv[process.argv.indexOf("--id") + 1];
    const out = process.argv[process.argv.indexOf("--out") + 1];
    const item = discoverNativeTestFile(
      path.resolve(options.rootDir, file),
    ).prophecies.find(
      (entry) =>
        entry.id === id ||
        `${file}::${entry.id.split("::").pop() ?? entry.id}` === id,
    );
    if (!item) process.exit(2);
    fs.mkdirSync(out, { recursive: true });
    const envelope = {
      prophecyId: id,
      suiteName: item.suiteName,
      name: item.name,
      foretell: { reason: item.foretell.reason },
      phase: "before-doom",
    } as const;
    fs.writeFileSync(
      path.join(out, "envelope.json"),
      `${JSON.stringify(envelope, null, 2)}\n`,
      "utf8",
    );
    const mod = await import(path.resolve(options.rootDir, file));
    const root = mod.default;
    const children = root?.children ?? [];
    for (const child of children) {
      if (child?.__tag !== "Prophecy" || child?.props?.name !== item.name)
        continue;
      const body = child.children.find((x: unknown) => typeof x === "function");
      if (typeof body === "function") await Promise.resolve(body());
    }
    return;
  }
  if (options.mode === "doom") {
    if (options.list) {
      const listed = listNativeProphecies({ rootDir: options.rootDir });
      const report = createNativeDoomReport({
        prophecies: listed.prophecies.map((p) => ({
          id: p.id,
          name: p.name,
          status: "passed",
        })),
        diagnostics: listed.diagnostics,
      });
      process.stdout.write(
        options.json
          ? formatNativeDoomJsonReport(report)
          : formatNativeDoomTextReport(report),
      );
      process.exit(nativeDoomExitCode(report));
    }
    const run = await runNativeProphecies({
      rootDir: options.rootDir,
      filter: options.filter,
      outDir: options.out,
    });
    const report = createNativeDoomReport(run);
    process.stdout.write(
      options.json
        ? formatNativeDoomJsonReport(report)
        : formatNativeDoomTextReport(report),
    );
    process.exit(nativeDoomExitCode(report));
  }
  if (options.mode === "artifact") {
    if (options.list) {
      const listed = await listNativeArtifacts({ rootDir: options.rootDir });
      const report = createNativeArtifactReport({
        artifacts: listed.artifacts.map((a) => ({
          id: a.id,
          name: a.name,
          status: "passed",
        })),
        diagnostics: listed.diagnostics,
      });
      process.stdout.write(
        options.json
          ? formatNativeArtifactJsonReport(report)
          : formatNativeArtifactTextReport(report),
      );
      process.exit(nativeArtifactExitCode(report));
    }
    const result = await runNativeArtifacts({
      rootDir: options.rootDir,
      filter: options.filter,
      artifactRoot: options.out,
    });
    const report = createNativeArtifactReport(result);
    process.stdout.write(
      options.json
        ? formatNativeArtifactJsonReport(report)
        : formatNativeArtifactTextReport(report),
    );
    process.exit(nativeArtifactExitCode(report));
  }
  if (options.mode === "bench") {
    if (options.list) {
      const listed = listNativeBenchmarks({ rootDir: options.rootDir });
      const report = createNativeBenchmarkReport({
        benchmarks: listed.benchmarks.map((b) => ({
          id: b.id,
          name: b.name,
          status: "passed",
          iterations: b.iterations,
          warmup: b.warmup,
        })),
        diagnostics: listed.diagnostics,
      });
      process.stdout.write(
        options.json
          ? formatNativeBenchmarkJsonReport(report)
          : formatNativeBenchmarkTextReport(report),
      );
      process.exit(nativeBenchmarkExitCode(report));
    }
    const run = await runNativeBenchmarks({
      rootDir: options.rootDir,
      filter: options.filter,
    });
    const report = createNativeBenchmarkReport(run);
    process.stdout.write(
      options.json
        ? formatNativeBenchmarkJsonReport(report)
        : formatNativeBenchmarkTextReport(report),
    );
    process.exit(nativeBenchmarkExitCode(report));
  }

  if (options.list) {
    const listed = await listNativeTests({ rootDir: options.rootDir });
    const report = createNativeTestReport({
      results: listed.tests.map((t) => ({
        id: t.id,
        name: t.name,
        status: "passed",
      })),
      diagnostics: listed.diagnostics,
    });
    process.stdout.write(
      options.json
        ? formatNativeTestJsonReport(report)
        : formatNativeTestTextReport(report),
    );
    process.exit(nativeTestExitCode(report));
  }
  const runResult = await runNativeTestFiles({
    rootDir: options.rootDir,
    filter: options.filter,
    updateSnapshots: options.updateSnapshots,
    batch: options.batch,
    files: options.files,
  });
  const report = createNativeTestReport(runResult);
  if (options.json) {
    process.stdout.write(formatNativeTestJsonReport(report));
  } else if (options.compact) {
    process.stdout.write(formatNativeTestCompactTextReport(report));
  } else {
    process.stdout.write(formatNativeTestTextReport(report));
  }
  process.exit(nativeTestExitCode(report));
}

main().catch((error: unknown) => {
  process.stderr.write(`TSPACK_TEST_XTEST_FAILED: ${String(error)}\n`);
  process.exit(1);
});
