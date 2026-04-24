# Changelog

All notable changes to hush are documented here.

The format follows [Keep a Changelog](https://keepachangelog.com/en/1.1.0/)
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

### Added
- Initial public release.
- CLI with `scan`, `rules`, `version` subcommands.
- Top level Go library at `github.com/valllabh/hush` with `New`, `Default`,
  `Scanner`, `Options`, `Finding`, `ModelVersion`. One import for the common
  path; `pkg/scanner`, `pkg/classifier`, `pkg/extractor` remain available for
  advanced use.
- Embedded BitNet ONNX model (`hush-model-v1`) and tokenizer.
- Example gallery covering CI, data masking, LLM proxying, and more.
