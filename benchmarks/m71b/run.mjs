import { execFileSync, spawnSync } from "node:child_process";
import { mkdirSync, statSync, writeFileSync } from "node:fs";
import { cpus, platform, release, arch } from "node:os";
import { dirname, resolve } from "node:path";
import { fileURLToPath } from "node:url";

const benchmarkRoot = dirname(fileURLToPath(import.meta.url));
const repositoryRoot = resolve(benchmarkRoot, "../..");
const fixtureRoot = resolve(repositoryRoot, "fixtures/perry-hotpath-m71b");
const sourcePath = resolve(fixtureRoot, "src/hot/bench.ts");
const perryPath = resolve(fixtureRoot, "dist/hotpath.exe");
const artifactPath = resolve(repositoryRoot, "artifacts/m71b/perf-results.json");

function runJson(executable, args, cwd) {
	const completed = spawnSync(executable, args, { cwd, encoding: "utf8", maxBuffer: 16 * 1024 * 1024 });
	if (completed.status !== 0) {
		throw new Error(`${executable} failed (${completed.status}): ${completed.stderr}`);
	}
	return JSON.parse(completed.stdout.trim());
}

function percentile(sorted, fraction) {
	const index = Math.min(sorted.length - 1, Math.ceil(sorted.length * fraction) - 1);
	return sorted[index];
}

function statistics(samples) {
	const sorted = [...samples].sort((left, right) => left - right);
	const mean = sorted.reduce((sum, value) => sum + value, 0) / sorted.length;
	const variance = sorted.reduce((sum, value) => sum + ((value - mean) ** 2), 0) / sorted.length;
	return {
		minMs: sorted[0],
		medianMs: percentile(sorted, 0.5),
		p95Ms: percentile(sorted, 0.95),
		maxMs: sorted[sorted.length - 1],
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

function measureProbe(executable, baseArgs, bytes) {
	const payload = bytes === 0 ? "" : JSON.stringify({ value: "x".repeat(Math.max(0, bytes - 12)) });
	const samples = [];
	let result;
	for (let index = 0; index < 11; index += 1) {
		const started = performance.now();
		result = runJson(executable, [...baseArgs, "--probe", payload], fixtureRoot);
		samples.push(performance.now() - started);
	}
	return { requestedBytes: bytes, returnedBytes: result.bytes, samplesMs: samples, statistics: statistics(samples) };
}

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
const nodeRows = [];
const perryRows = [];
for (const workloadName of workloadNames) {
	const nodeReport = runJson(process.execPath, [sourcePath, "--workload", workloadName], fixtureRoot);
	const perryReport = runJson(perryPath, ["--workload", workloadName], fixtureRoot);
	nodeRows.push(...summarize("Node/V8", nodeReport));
	perryRows.push(...summarize("Perry", perryReport));
}
const nodeByName = new Map(nodeRows.map((row) => [row.workload, row]));
const correctness = perryRows.map((row) => ({
	workload: row.workload,
	match: row.correct && nodeByName.get(row.workload)?.checksum === row.checksum,
	nodeChecksum: nodeByName.get(row.workload)?.checksum,
	perryChecksum: row.checksum,
}));
for (const row of perryRows) {
	const parity = correctness.find((candidate) => candidate.workload === row.workload);
	if (!parity?.match) {
		row.sampleCount = 0;
		row.statistics = null;
		row.samplesMs = [];
		row.correct = false;
	}
}

const payloadSizes = [0, 256, 4096, 16384];
const result = {
	schemaVersion: 1,
	generatedAt: new Date().toISOString(),
	environment: {
		os: `${platform()} ${release()}`,
		architecture: arch(),
		cpu: cpus()[0]?.model ?? "unknown",
		logicalProcessors: cpus().length,
		powerMode: "Balanced",
		node: process.version,
		perry: "0.5.1220",
		perryCommitAudited: "f9890759c53f29449ac97320af615757a9111ff2",
		copelandCommit: execFileSync("git", ["-C", resolve(repositoryRoot, "../Copeland"), "rev-parse", "HEAD"], { encoding: "utf8" }).trim(),
		dotnetRuntime: "10.0.11",
		bun: null,
	},
	methodology: {
		warmupPerWorkload: 5,
		measuredSamplesPerWorkload: 11,
		steadyState: "all samples are timed inside one process per implementation",
		correctness: "exact integer checksum parity; invalid rows retain no performance samples",
	},
	correctness,
	workloads: [...nodeRows, ...perryRows],
	boundary: {
		description: "fresh-process executable plus JSON argv parse/normalize/checksum; includes startup",
		node: payloadSizes.map((bytes) => measureProbe(process.execPath, [sourcePath], bytes)),
		perry: payloadSizes.map((bytes) => measureProbe(perryPath, [], bytes)),
	},
	artifacts: {
		perryExecutableBytes: statSync(perryPath).size,
	},
};

mkdirSync(dirname(artifactPath), { recursive: true });
writeFileSync(artifactPath, `${JSON.stringify(result, null, 2)}\n`);
console.log(artifactPath);
