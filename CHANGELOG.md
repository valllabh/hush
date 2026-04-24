# Changelog

All notable changes to hush are documented here.

The format follows [Keep a Changelog](https://keepachangelog.com/en/1.1.0/)
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

### Added
- **Pure Go inference runtime (`pkg/native`)**. Select at build time with
  `go build -tags=native ./cmd/hush`. Zero CGO, zero libonnxruntime
  dependency, a fully static single binary. Embedded int8 model
  (`hush-model-v1.int8.hbin`, 80 MB) shipped inside the package. Matches
  ORT numerics within 3e-6 on realistic inputs, 0.011 vs fp32 on the int8
  path. End-to-end throughput comparable to or faster than the ORT
  default on mixed workloads.
- `pkg/native.Scorer`, `NewBundledScorer()`, `LoadScorer`, `LoadScorerReader`
  for library users who want direct native access.
- `PERF.md` tracks kernel progression, end-to-end numbers, and numeric
  correctness per iteration.

### Changed
- CLI now selects its classifier backend through `pkg/scanner.DefaultScorerFactory`
  registered by `pkg/bundled`. The `!native` build tag wires ORT (default);
  the `native` tag wires the pure Go runtime. `cmd/hush` no longer imports
  `pkg/classifier` directly.

## [0.1.0] - initial public release

### Added
- Initial public release.
- CLI with `scan`, `rules`, `version` subcommands.
- Top level Go library at `github.com/valllabh/hush` with `New`, `Default`,
  `Scanner`, `Options`, `Finding`, `ModelVersion`. One import for the common
  path; `pkg/scanner`, `pkg/classifier`, `pkg/extractor` remain available for
  advanced use.
- Embedded BitNet ONNX model (`hush-model-v1`) and tokenizer.
- Example gallery covering CI, data masking, LLM proxying, and more.
