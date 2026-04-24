# Changelog

All notable changes to hush are documented here.

The format follows [Keep a Changelog](https://keepachangelog.com/en/1.1.0/)
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

## [0.1.2] - 2026-04-24

### Changed
- **Pure Go runtime is now the default.** No CGO, no libonnxruntime,
  no shared library, no runtime dependency of any kind. Just a static
  binary with the int8 BitNet model embedded inside.
- The ORT-backed path (`pkg/classifier`, `libonnxruntime`) is gated
  behind `-tags=ort` and kept only for numeric equivalence testing.
  Shipping binaries no longer include ORT code.
- Release pipeline simplified: pure-Go `CGO_ENABLED=0` build per
  OS/arch, no libonnxruntime download, no lib bundling, no INSTALL.md
  wrapping. Archive is `hush` + docs.

### Removed
- `libonnxruntime` dependency from the default build.
- `lib/` directory from release archives.
- `ONNXRUNTIME_LIB` environment variable requirement.

## [0.1.1] - 2026-04-24

### Added
- **Pure Go inference runtime (`pkg/native`)**. Initially opt-in via
  `go build -tags=native ./cmd/hush`. Embedded int8 model
  (`hush-model-v1.int8.hbin`, 80 MB). Matches ORT numerics within 3e-6
  on realistic inputs, 0.011 vs fp32 on the int8 path.
- `pkg/native.Scorer`, `NewBundledScorer()`, `LoadScorer`, `LoadScorerReader`.
- `PERF.md` tracks kernel progression and end-to-end numbers per iteration.

### Changed
- CLI selects its classifier backend through `pkg/scanner.DefaultScorerFactory`
  registered by `pkg/bundled`. `cmd/hush` no longer imports `pkg/classifier`
  directly.

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
