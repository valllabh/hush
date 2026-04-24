# Performance

Hush tracks end-to-end and kernel-level numbers for every iteration so
regressions are obvious and wins are visible. All numbers are from an
Apple M4 Pro unless noted; CI on ubuntu-latest is noted separately when
it differs meaningfully.

Repro:
- library benches: `go test -bench=. -benchmem -run=^$ -benchtime=3s ./pkg/native/`
- CLI perf: `HUSH_PERF=1 go test -run='^TestPerf_' -v ./cmd/hush/cli/`

## Headline (v0.1.0 → latest unreleased)

| metric                              | v0.1.0 (ORT) | unreleased (native) | delta |
| ----------------------------------- | ------------ | ------------------- | ----- |
| CLI startup (`hush version`)        | 1.25 s       | TBD                 |       |
| scan 100 files, with model          | 540 ms       | TBD                 |       |
| scan 100 files, `--model-off`       | 18 ms        | 18 ms               | same  |
| scan 1000 files, `--model-off`      | 40 ms        | 40 ms               | same  |
| RSS with classifier loaded          | 441 MiB      | TBD                 |       |
| Per-classifier-call latency         | ~13 ms       | 27-29 ms            | TBD\* |
| Binary size                         | 80 MB        | TBD                 |       |
| Requires libonnxruntime at runtime  | yes          | no                  |       |

\* The pure-Go classifier call is slower per invocation than ORT but
starts 25x faster, so full CLI wall time is expected to improve sharply.
End-to-end numbers land when the native backend is wired as the default.

## Native runtime kernel progression

| change                                          | ns/op     | MB/op | allocs/op |
| ----------------------------------------------- | --------- | ----- | --------- |
| initial naive ikj matmul, T=384 padded          | 4,382 ms  | 254   | 513       |
| dynamic T trim (skip padded positions)          | 45.5 ms   | 4     | 510       |
| mr=4 register-tiled blocked matmul              | 28.7 ms   | 4     | 510       |
| int8 weights + eager dequant at load            | 27.2 ms   | 4     | 502       |

Cumulative: **~160x** faster than the initial straight translation.

## Numeric correctness

| input                                              | Go logits                   | ORT logits                 | max abs diff |
| -------------------------------------------------- | --------------------------- | -------------------------- | ------------ |
| synthetic IDs [0,100,200,2,...]                    | [0.5780999, -0.73651636]    | [0.57809895, -0.7365156]   | 1e-6         |
| tokenized `api_key = "[CAND]AKIA...[/CAND]"`       | [-5.3829885, 5.800204]      | [-5.382986, 5.8002005]     | 3.3e-6       |
| int8 model vs fp32 (same input)                    | —                           | —                          | 0.0128       |

Tolerances enforced in CI: 1e-4 for fp32 vs ORT, 5e-2 for int8 vs fp32.

## Model size

| artifact                          | size     |
| --------------------------------- | -------- |
| baseline_fp32.onnx                | 313 MB   |
| baseline_int8.onnx (ORT-exported) | 82 MB    |
| model.hbin fp32                   | 313 MB   |
| model_int8.hbin                   | 190 MB\* |

\* Word embeddings still fp32 (154 MB alone). Quantizing embeddings is
the next crank — target under 100 MB to enable `go:embed`.

## Test baseline (reproducible)

- `TestForwardMatchesORT` — synthetic input numeric match
- `TestForwardMatchesORTRealistic` — tokenized real input numeric match
- `TestInt8ForwardMatchesFP32` — int8 path vs fp32
- `BenchmarkForward` — full forward pass
- `BenchmarkForwardInt8` — int8 forward pass

See CHANGELOG.md for the feature story; this file is the numbers-only log.
