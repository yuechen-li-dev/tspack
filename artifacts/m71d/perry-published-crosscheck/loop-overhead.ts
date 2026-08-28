// Exact Perry v0.5.1220 benchmarks/suite/02_loop_overhead.ts workload.
const ITERATIONS = 100000000;
let sum = 0;

const start = Date.now();
for (let i = 0; i < ITERATIONS; i++) {
  sum = sum + 1;
}
const elapsed = Date.now() - start;

console.log("loop_overhead:" + elapsed);
console.log("sum:" + sum);
