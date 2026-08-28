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
const started = performance.now();
const results = seeds.map((seed) => compute(iterations, seed));
const kernelMs = performance.now() - started;
console.log(JSON.stringify({ results, kernelMs }));
