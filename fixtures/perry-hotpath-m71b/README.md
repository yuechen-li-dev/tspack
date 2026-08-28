# Perry hot-path M71b dogfood

This fixture keeps `src/app/**` under ordinary `tsc` ownership and assigns only
`src/hot/**` to Perry. The app consumes the declared native executable through
one coarse sidecar call; it never imports Perry-owned source.

`src/hot/bench.ts` is also the independent shared-source Node/Perry benchmark.
It warms each of ten workload families five times, takes eleven in-process
samples, and emits exact checksums with every distribution.
