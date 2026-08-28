function convolution(): number {
const width = 512;
const height = 512;
const channels = 4;
const input = new Uint8Array(width * height * channels);
const output = new Uint8Array(input.length);
for (let index = 0; index < input.length; index += 1) {
  input[index] = (index * 17 + (index >>> 5)) & 255;
}

const weights = new Int32Array([
  1, 4, 6, 4, 1,
  4, 16, 24, 16, 4,
  6, 24, 36, 24, 6,
  4, 16, 24, 16, 4,
  1, 4, 6, 4, 1,
]);

for (let y = 2; y < height - 2; y += 1) {
  for (let x = 2; x < width - 2; x += 1) {
    for (let channel = 0; channel < 3; channel += 1) {
      let sum = 0;
      let coefficient = 0;
      for (let ky = -2; ky <= 2; ky += 1) {
        for (let kx = -2; kx <= 2; kx += 1) {
          const source = ((y + ky) * width + x + kx) * channels + channel;
          sum += input[source] * weights[coefficient];
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

type Workload = {
  name: string;
  run: () => number;
};

type WorkloadResult = {
  name: string;
  warmup: number;
  samplesMs: number[];
  expectedChecksum: number;
  checksum: number;
  correct: boolean;
};

const workloads: Workload[] = [{ name: "convolution", run: convolution }];
let observation = 0;
function measure(workload: Workload): WorkloadResult {
  let checksum = workload.run();
  const expectedChecksum = checksum;
  let correct = true;
  observation ^= checksum;
  for (let index = 0; index < 5; index += 1) {
    checksum = workload.run();
    correct = correct && checksum === expectedChecksum;
    observation ^= checksum;
  }

  const samplesMs: number[] = [];
  for (let index = 0; index < 11; index += 1) {
    const started = performance.now();
    checksum = workload.run();
    correct = correct && checksum === expectedChecksum;
    samplesMs.push(performance.now() - started);
    observation ^= checksum;
  }
  return { name: workload.name, warmup: 5, samplesMs, expectedChecksum, checksum, correct };
}

console.log(JSON.stringify({ observation, workloads: [measure(workloads[0])] }));
