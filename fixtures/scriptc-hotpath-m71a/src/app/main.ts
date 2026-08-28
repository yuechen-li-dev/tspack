import { spawnSync } from "node:child_process";
import { fileURLToPath } from "node:url";

function compute(iterations: number, seed: number): number {
	let state = seed | 0;
	let checksum = 0;
	for (let index = 0; index < iterations; index++) {
		state ^= state << 13;
		state ^= state >>> 17;
		state ^= state << 5;
		checksum = (checksum + (state & 0xffff)) | 0;
	}
	return checksum >>> 0;
}

const iterations = 5000000;
const seeds = [1, 7, 42, 65537];

// Warm V8 before measuring the ordinary TypeScript baseline.
for (const seed of seeds) {
	compute(10000, seed);
}
const baselineStarted = performance.now();
const baseline = seeds.map((seed) => compute(iterations, seed));
const baselineMs = performance.now() - baselineStarted;

// This is the explicit compiler boundary: a native artifact dependency and a
// single batched JSON response, never an import of ScriptC-owned source.
const executable = fileURLToPath(new URL("../hotpath.exe", import.meta.url));
const boundaryStarted = performance.now();
const completed = spawnSync(executable, [], { encoding: "utf8" });
const boundaryMs = performance.now() - boundaryStarted;
if (completed.status !== 0) {
	throw new Error(`ScriptC sidecar failed (${completed.status}): ${completed.stderr}`);
}
const accelerated = JSON.parse(completed.stdout) as { results: number[]; kernelMs: number };
if (JSON.stringify(accelerated.results) !== JSON.stringify(baseline)) {
	throw new Error(
		`parity mismatch: node=${JSON.stringify(baseline)} scriptc=${JSON.stringify(accelerated.results)}`,
	);
}

console.log(
	JSON.stringify({
		parity: true,
		results: baseline,
		baselineMs,
		scriptcKernelMs: accelerated.kernelMs,
		boundaryMs,
		boundaryOverheadMs: boundaryMs - accelerated.kernelMs,
	}),
);
