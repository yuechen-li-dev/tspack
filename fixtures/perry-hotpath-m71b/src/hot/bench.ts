type Workload = {
	name: string;
	run: () => number;
};

// Keep this source compiler-neutral: Node and Perry execute the same kernels.

type WorkloadResult = {
	name: string;
	warmup: number;
	samplesMs: number[];
	checksum: number;
	correct: boolean;
};

const warmupCount = 5;
const sampleCount = 11;
let observation = 0;

function integerLoop(): number {
	let value = 0;
	for (let index = 0; index < 12_000_000; index += 1) {
		value = (value + ((index * 31) ^ (index >>> 3))) | 0;
	}
	return value;
}

function floatingPointKernel(): number {
	let value = 0.25;
	for (let index = 1; index < 2_000_000; index += 1) {
		value += Math.sin(index * 0.00001) * 0.5 + Math.sqrt((index % 97) + 1);
	}
	return Math.floor(value * 1000);
}

function convolution(): number {
	const width = 512;
	const height = 512;
	const channels = 4;
	const input = new Uint8Array(width * height * channels);
	const output = new Uint8Array(input.length);
	for (let index = 0; index < input.length; index += 1) {
		input[index] = (index * 17 + (index >>> 5)) & 255;
	}
	const coefficients = new Int32Array([1, 4, 6, 4, 1, 4, 16, 24, 16, 4, 6, 24, 36, 24, 6, 4, 16, 24, 16, 4, 1, 4, 6, 4, 1]);
	for (let y = 2; y < height - 2; y += 1) {
		for (let x = 2; x < width - 2; x += 1) {
			for (let channel = 0; channel < 3; channel += 1) {
				let sum = 0;
				let coefficient = 0;
				for (let ky = -2; ky <= 2; ky += 1) {
					for (let kx = -2; kx <= 2; kx += 1) {
						const source = ((y + ky) * width + x + kx) * channels + channel;
						sum += input[source] * coefficients[coefficient];
						coefficient += 1;
					}
				}
				output[(y * width + x) * channels + channel] = sum >>> 8;
			}
		}
	}
	let checksum = 0;
	for (let index = 0; index < output.length; index += 4093) {
		checksum = (checksum * 33 + output[index]) | 0;
	}
	return checksum;
}

function primitiveArray(): number {
	const values = new Int32Array(1_000_000);
	for (let pass = 0; pass < 12; pass += 1) {
		for (let index = 0; index < values.length; index += 1) {
			values[index] = values[index] + ((index + pass) & 255);
		}
	}
	return values[999_983];
}

function records(): number {
	const values: { x: number; y: number; z: number }[] = [];
	for (let index = 0; index < 200_000; index += 1) {
		values.push({ x: index, y: index * 2, z: index & 31 });
	}
	let total = 0;
	for (let pass = 0; pass < 8; pass += 1) {
		for (let index = 0; index < values.length; index += 1) {
			total = (total + values[index].x + values[index].y - values[index].z) | 0;
		}
	}
	return total;
}

function allocationChurn(): number {
	let total = 0;
	for (let pass = 0; pass < 20; pass += 1) {
		const values: { id: number; next: number; live: boolean }[] = [];
		for (let index = 0; index < 80_000; index += 1) {
			values.push({ id: index, next: index + 1, live: (index & 1) === 0 });
		}
		for (let index = 0; index < values.length; index += 97) {
			total += values[index].live ? values[index].next : values[index].id;
		}
	}
	return total | 0;
}

function byteHash(): number {
	const bytes = new Uint8Array(4_000_000);
	for (let index = 0; index < bytes.length; index += 1) {
		bytes[index] = (index * 13 + 91) & 255;
	}
	let hash = 0x811c9dc5;
	for (let index = 0; index < bytes.length; index += 1) {
		hash ^= bytes[index];
		hash = Math.imul(hash, 0x01000193);
	}
	return hash | 0;
}

function jsonRoundtrip(): number {
	const values: { id: number; name: string; score: number; active: boolean }[] = [];
	for (let index = 0; index < 12_000; index += 1) {
		values.push({ id: index, name: `item_${index}`, score: index * 3.125, active: (index & 1) === 0 });
	}
	const blob = JSON.stringify(values);
	let size = 0;
	for (let pass = 0; pass < 8; pass += 1) {
		const parsed = JSON.parse(blob);
		size += JSON.stringify(parsed).length;
	}
	return size;
}

function branchHeavy(): number {
	let state = 0x12345678;
	let total = 0;
	for (let index = 0; index < 16_000_000; index += 1) {
		state = (Math.imul(state, 1664525) + 1013904223) | 0;
		if ((state & 7) === 0) total += index;
		else if ((state & 3) === 0) total -= state;
		else total ^= state;
	}
	return total | 0;
}

function mix(value: number, salt: number): number {
	return (Math.imul(value ^ salt, 33) + (value >>> 7)) | 0;
}

function abstractionHeavy(): number {
	let value = 7;
	for (let index = 0; index < 8_000_000; index += 1) {
		value = mix(value, index);
	}
	return value;
}

const workloads: Workload[] = [
	{ name: "integer-loop-m71a", run: integerLoop },
	{ name: "floating-point", run: floatingPointKernel },
	{ name: "image-convolution-5x5", run: convolution },
	{ name: "primitive-array-rw", run: primitiveArray },
	{ name: "array-of-records", run: records },
	{ name: "allocation-churn", run: allocationChurn },
	{ name: "byte-hash", run: byteHash },
	{ name: "json-roundtrip", run: jsonRoundtrip },
	{ name: "branch-heavy", run: branchHeavy },
	{ name: "function-calls", run: abstractionHeavy },
];

function measure(workload: Workload): WorkloadResult {
	let checksum = workload.run();
	const expectedChecksum = checksum;
	let correct = true;
	observation ^= checksum;
	for (let index = 0; index < warmupCount; index += 1) {
		checksum = workload.run();
		correct = correct && checksum === expectedChecksum;
		observation ^= checksum;
	}
	const samplesMs: number[] = [];
	for (let index = 0; index < sampleCount; index += 1) {
		const started = performance.now();
		checksum = workload.run();
		correct = correct && checksum === expectedChecksum;
		const elapsed = performance.now() - started;
		samplesMs.push(elapsed);
		observation ^= checksum;
	}
	return { name: workload.name, warmup: warmupCount, samplesMs, checksum, correct };
}

function runBenchmarks(selectedName: string): void {
	const results: WorkloadResult[] = [];
	for (const workload of workloads) {
		if (selectedName.length > 0 && workload.name !== selectedName) {
			continue;
		}
		results.push(measure(workload));
	}
	console.log(JSON.stringify({ schemaVersion: 1, observation, workloads: results }));
}

function runProbe(payload: string): void {
	if (payload.length === 0) {
		console.log(JSON.stringify({ bytes: 0, checksum: 0 }));
		return;
	}
	const parsed = JSON.parse(payload);
	const normalized = JSON.stringify(parsed);
	let checksum = 0;
	for (let index = 0; index < normalized.length; index += 1) {
		checksum = (checksum + normalized.charCodeAt(index)) | 0;
	}
	console.log(JSON.stringify({ bytes: normalized.length, checksum }));
}

if (process.argv[2] === "--probe") {
	runProbe(process.argv[3] || "");
} else {
	const selectedName = process.argv[2] === "--workload" ? (process.argv[3] || "") : "";
	runBenchmarks(selectedName);
}
