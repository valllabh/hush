# Performance

Hush tracks end-to-end and kernel-level numbers for every iteration so
regressions are obvious and wins are visible. All numbers are from an
Apple M4 Pro unless noted.

Repro:
- library benches: `go test -bench=. -benchmem -run=^$ -benchtime=3s ./pkg/native/`
- CLI perf: `HUSH_PERF=1 go test -run='^TestPerf_' -v ./cmd/hush/cli/` (default)
            `HUSH_PERF=1 go test -tags=native -run='^TestPerf_' -v ./cmd/hush/cli/` (pure Go)

## Rule coverage

| version | secret rules | pii rules | total |
| ------- | ------------ | --------- | ----- |
| v0.1.2  | 9            | 0         | 9     |
| v0.1.3  | 68           | 24        | 92    |

## Headline (v0.1.0 ORT default vs v0.1.2+ pure Go default)

Cold start, 50 mixed files (25 clean + 25 with planted AWS keys):

| metric                              | v0.1.0 ORT   | v0.1.2 native | delta   |
| ----------------------------------- | ------------ | ------------- | ------- |
| Binary size                         | 92 MB        | 93 MB         | +1 MB   |
| Embedded model                      | 82 MB ONNX   | 84 MB int8 hbin | --    |
| Requires libonnxruntime at runtime  | **yes**      | **no**        | win    |
| Cold-start scan, 50 files mixed     | 0.56 s       | 0.53 s        | -5%    |
| Cold-start scan, 50 files x 5 cands | 0.72 s       | 0.62 s        | **-14%** |
| RSS (ORT or native classifier)      | ~190-440 MB  | ~460-780 MB   | higher\*|

\* Native pays RAM for eager dequant (int8 weights -> fp32 at load).
Trade: no libonnxruntime, simpler deployment, comparable speed.

## Native runtime kernel progression

| change                                          | ns/op     | MB/op | allocs/op |
| ----------------------------------------------- | --------- | ----- | --------- |
| initial naive ikj matmul, T=384 padded          | 4,382 ms  | 254   | 513       |
| dynamic T trim (skip padded positions)          | 45.5 ms   | 4     | 510       |
| mr=4 register-tiled blocked matmul              | 28.7 ms   | 4     | 510       |
| int8 weights + eager dequant at load            | 27.2 ms   | 4     | 502       |
| pre-packed B + 4x4 register-tile kernel         | 15.3 ms   | 1.8   | 502       |
| per-forward tensor arena + sync.Pool reuse      | 15.1 ms   | 0.015 | 76        |

Cumulative: **~290x** faster than the initial straight translation.

## Numeric correctness

| input                                              | Go logits                   | ORT logits                 | max abs diff |
| -------------------------------------------------- | --------------------------- | -------------------------- | ------------ |
| synthetic IDs [0,100,200,2,...]                    | [0.5780999, -0.73651636]    | [0.57809895, -0.7365156]   | 1e-6         |
| tokenized `api_key = "[CAND]AKIA...[/CAND]"`       | [-5.3829885, 5.800204]      | [-5.382986, 5.8002005]     | 3.3e-6       |
| int8 model vs fp32 (same input)                    | —                           | —                          | 0.011        |

Tolerances enforced in CI: 1e-4 for fp32 vs ORT, 5e-2 for int8 vs fp32.

## Model size

| artifact                          | size     | notes                                |
| --------------------------------- | -------- | ------------------------------------ |
| baseline_fp32.onnx                | 313 MB   | research only, too big to ship       |
| baseline_int8.onnx (ORT-exported) | 82 MB    | shipped in pkg/classifier (ORT path) |
| model.hbin fp32                   | 313 MB   | dev artifact                         |
| model_int8.hbin (matmuls only)    | 190 MB   | dev artifact                         |
| model_int8.hbin (+ embeddings)    | **80 MB**| **shipped in pkg/native**            |

## Test baseline (reproducible)

- `TestForwardMatchesORT` — synthetic input numeric match, 1e-4
- `TestForwardMatchesORTRealistic` — tokenized real input, 3.3e-6
- `TestInt8ForwardMatchesFP32` — int8 vs fp32, 5e-2
- `TestBundledScorer` — embedded model end to end, AKIA scores 0.9999
- `BenchmarkForward` — full forward pass
- `BenchmarkForwardInt8` — int8 forward pass
- `TestPerf_*` (HUSH_PERF=1) — CLI wall time + RSS (honours build tag)

## Build commands

```
# default: pure Go, no CGO, no libonnxruntime, static binary
go build ./cmd/hush

# testing only: ORT path for numeric equivalence diffs
go build -tags=ort ./cmd/hush
```

## Next goals (Tier 1 optimizations)

Queued and tracked here so each lands with numbers:

1. **Weight pre-packing at load time** — target -20%. Store each weight
   in the exact layout `matmulBlocked` consumes (contiguous B rows by
   the `mr=4` tile). Eliminates strided access inside the hot loop.
2. **Tensor arena / pool** — target -10% + much lower GC pressure.
   Replace the ~500 allocs/forward with a reused workspace sized for
   the max-seq-len layer. Cuts MB/op from 4 to ~0.5.
3. ~~**Transpose-free attention**~~ — **tried and reverted on T=4 bench.**
   Implemented stride-aware `Tensor` + zero-copy `TransposeView` and a
   strided `BatchMatMul` path. Numeric tests stayed green but the wall
   speedup on the synthetic benchmark was only 1.3% (15.1 ms -> 14.9 ms)
   and allocs went **up** (76 -> 100) because the strided kernel's
   inner loop is slower per-element than the contiguous blocked kernel
   and stride-slice bookkeeping added small allocations. With T=4 the
   full transposes were already cheap (tiny tensors) so there was
   nothing meaningful to save. Leave this on the table for longer
   sequences (e.g. streaming / batch >1 workloads) where the ratio
   tips the other way.
4. **NEON / AVX assembly inner loop** — target -50% on top. Borrow
   `gonum/internal/asm/f32` kernels for the `fma`-style inner product.

Combined target: **27 ms → 5-7 ms per forward**, beating the ORT path
on per-call and crushing it on startup.

See CHANGELOG.md for the feature story; this file is the numbers-only log.
