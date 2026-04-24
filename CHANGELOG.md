# Changelog

All notable changes to hush are documented here.

The format follows [Keep a Changelog](https://keepachangelog.com/en/1.1.0/)
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

## [0.1.4] - 2026-04-24

### Performance
- **Weight pre-packing**: pack each matmul weight into an `[N/4, K, 4]`
  panel layout at load time, plus a 4×4 register-resident kernel that
  keeps C in 16 scalar registers across the full K reduction.
  BenchmarkForward: **27.6 ms → 15.3 ms (-44%)**.
- **Tensor arena + sync.Pool**: per-forward arena reuses float32 buffers,
  tensor structs, and shape slices across iterations. Allocs per forward
  **502 → 76 (-85%)**, memory **1.77 MB → 15 KB per op (-99%)**. Wall
  time another -1.7%.
- Net: **~45% faster, ~118x less memory pressure** vs v0.1.3 (same
  numeric correctness, 1e-4 vs ORT, 5e-2 int8 vs fp32).

### Tried and parked
- Transpose-free attention via stride-aware `Tensor` + zero-copy view.
  Only 1.3% wall win at T=4 (synthetic bench) and allocs regressed.
  Reverted; worth revisiting for long-sequence or batched workloads.

## [0.1.3] - 2026-04-24

### Added
- **Rule set expanded from 9 to 92 built-in patterns**: ports the active
  gitleaks ruleset (credited in `rules.json`), covering cloud providers
  (AWS access + secret + session, GCP service-account + OAuth + API,
  Azure connection string / SAS / account key, DigitalOcean, Linode,
  Alibaba), AI providers (OpenAI, Anthropic, HuggingFace, Cohere),
  SaaS (Twilio, SendGrid, Mailgun, PagerDuty, Datadog, New Relic,
  Cloudflare, Shopify, Square, PayPal, Atlassian, Okta, Auth0, Heroku),
  webhooks (Slack, Discord, Telegram, Teams), database URIs, package
  registry tokens, CI PATs, and more.
- **PII detection**: 24 new PII rules (SSN, SIN, NIN, Aadhaar, PAN,
  CPF, credit card shape, IBAN, email, phone, IP, MAC, passport, etc.).
  Tagged `"type": "pii"` in the rule file.
- **`--detect` flag**: accepts `secrets`, `pii`, `both` (comma combos
  also ok). Default is `both`. Filters the active rule set by type.
- Every rule carries a `type` field (`"secret"` or `"pii"`); default is
  `"secret"` for compatibility with rule files missing the field.

### Fixed
- `.hush.yaml` in repo root used an old `ext: [py, env, ...]` shape
  where bare extension names got misinterpreted as directory globs under
  the new `file-include` classifier, silently excluding every file from
  directory scans run inside this repo. Switched to `*.py` form.

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
