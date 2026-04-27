# Changelog

All notable changes to hush are documented here.

The format follows [Keep a Changelog](https://keepachangelog.com/en/1.1.0/)
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

## [0.1.13] - 2026-04-27

### Fixed
- **URL fragments no longer flagged as secrets.** The high-entropy
  fallback in `pkg/extractor` was matching long alphanumeric runs
  inside URLs (badge URLs in README.md, docs links in CI yaml,
  pkg.go.dev references). Caught by today's perf-bench run on
  `sirupsen/logrus`: 65 false positives, the majority of them URL
  fragments. The hybrid pipeline now drops high-entropy hits whose
  span starts with `http://`/`https://` or sits fully inside a URL on
  the same line. Strong-signal rules (PEM blocks, AWS prefixes, etc.)
  bypass this via `isHighTrustRule`.
- **Plain code identifiers no longer flagged as PII.** The v2 head
  occasionally tagged a CamelCase fragment sliced out of a longer
  identifier (`evelText` from `PadLevelText` in a CHANGELOG bullet)
  as PII. Real PII spans contain an `@`, a digit, or a separator.
  Spans that are 3-16 letters with no other character class are now
  dropped from both regex-emit and model-only-emit paths.

### Added
- Three new corpus fixtures under
  `pkg/scanner/testdata/corpus/negatives/`:
  - `url_in_readme.txt` — markdown badge URLs
  - `url_in_ci_yaml.txt` — GitHub Actions docs links and `uses:` refs
  - `code_identifiers.txt` — CHANGELOG with `PadLevelText`-style names
  All paired with `<name>.expected.json` declaring zero spans, so the
  build gate fails any future regression in these classes.

## [0.1.12] - 2026-04-27

### Fixed
- **`hush detect <dir>` now walks the directory.** Previous releases
  treated every positional arg as a file and called `os.ReadFile`,
  failing with `is a directory` on the most natural CLI invocation
  (`hush detect ./my-repo`). Caught by an EC2/Fargate perf-bench run
  on 2026-04-27 against `/sample/logrus`.
  - Reuses `internal/walker.Walk` so `hush detect` and `hush scan`
    share one central skip list (`internal/walker.DefaultSkipDirs`,
    28 entries: `.git`, `node_modules`, `vendor`, `dist`, `build`,
    `.venv`, `__pycache__`, etc.), the binary-file sniff, and the
    10 MB per-file size cap.
  - Stops hardcoding skip rules per subcommand.

### Added
- `TestIntegration_DetectWalksDirectory` regression test asserting
  detect on a directory exits 1, finds the dirty file in a subdir,
  and skips `node_modules`. The test now blocks any future regression.

## [0.1.11] - 2026-04-25

Robustness pass: hush now behaves well on real-world inputs (giant
files, embedded blobs, concurrent goroutines, streams larger than RAM,
encoded secret blobs).

### Added
- **#5 Encoded-secret pre-pass.** After the regex sweep, every
  base64-shaped token (length divisible by 4, valid base64 alphabet)
  is decoded once and re-checked against the rule set. When a known
  rule matches the decoded form (e.g. an AWS key smuggled inside a
  base64 string literal), the original encoded span is emitted with
  a synthetic `encoded_<rule>` rule name. Total decode work is capped
  at 10x the input length to avoid pathological all-base64 inputs.
- **#12 Large-file streaming.** Files larger than 50 MB now flow
  through `(*Scanner).ScanReader` instead of `os.ReadFile`. Per-file
  model invocations are capped at 200 chunks (~200 MB of streaming
  work) so a 10 GB log cannot pin a worker indefinitely.
- **#15 Chunked `ScanReader`.** `(*Scanner).ScanReader` now reads in
  1 MB chunks with a 4 KB carry-over so peak RSS stays bounded
  regardless of input size. Findings whose absolute offset matches
  across chunks are deduped. Verified with a 4 MB junk stream
  carrying a real AWS key and with a key that straddles a 1 MB
  chunk boundary.

### Fixed
- **#9 Long base64 chunks in source.** `FindHighEntropySpans` now
  masks `data:image/...;base64,...` URIs and full
  `-----BEGIN CERTIFICATE----- ... -----END CERTIFICATE-----` blocks
  before entropy scoring. Inline images and embedded certs no
  longer flood the report with high-entropy false positives.
- **#14 Detector concurrency.** `*native.Detector.Detect` now
  serialises calls through an internal `sync.Mutex`. Library users
  wiring one Detector into a worker pool no longer race on the
  shared tensor buffers. Documented throughput tradeoff: callers
  needing parallel forward passes should construct N Detectors and
  round-robin across them. Regression test hammers a single
  Detector from 10 goroutines and asserts no panics + consistent
  span counts.

### Notes
The chunked path is bytes-aware, not token-aware. A multi-line PEM
that spans more than the 4 KB overlap will still be missed by the
regex extractor because the BEGIN/END boundary cannot fit in one
chunk. For now keep large PEM-bearing files under the 50 MB
threshold; future work: bump overlap dynamically when an unmatched
BEGIN block is seen.

## [0.1.10] - 2026-04-25

Quick wins from the failure-fixes plan: tighter example-marker scoping,
soft prefilter for pure-prose PII lines, looser phone and credit-card
shapes, full-block PEM capture, binary-file sniff in the walker,
mask-output edge-byte safety, and JSON safety for library callers.

### Fixed
- **#2 Misleading example comment.** `looksLikeExample` now checks only
  the candidate's own line plus a single immediately preceding
  comment-only line (no assignment-looking content). A misleading
  `// example key` comment far above a real-looking secret line no
  longer suppresses the finding.
- **#4 Phone area codes.** `phone_us` regex loosened from `[2-9]` to
  `[0-9]` for both the area code and the exchange. Numbers like
  `155-867-5309` and `055-123-4567` now match; the model decides
  whether the match is real PII.
- **#6 Multi-line PEM blocks.** `private_key_pem`, `x509_certificate`,
  and `pgp_private_key` now match the full BEGIN..END span via the
  RE2 `(?s)` flag instead of stopping at the BEGIN boundary. Verified
  against an RSA-4096-style block. v2 fusion path no longer drops PEM
  matches when the model labels the body as noise (high-trust rule
  list).
- **#16 Credit cards with separators.** Visa, Mastercard, Amex,
  Discover, and JCB regexes accept `[ -]?` between every 4-digit
  group. `4532 0151 1283 0366` now matches.
- **#18 Mask-output edge bytes.** `MaskText` snaps every finding's
  `[Start,End]` outward to the regex extractor's match at that
  position before applying the placeholder. A model-narrow span no
  longer leaves leading or trailing bytes of the underlying secret
  visible in the masked output.

### Added
- **#3 Soft prefilter for pure-prose PII.** The detector prefilter
  now synthesizes "soft" candidates for two-cap-word names
  (`Vallabh Joshi`, `Linus Torvalds`) and US street suffixes near
  numbers (`123 Main St`, `742 Evergreen Avenue`). Without these
  the model never saw lines that carried only contextual PII.
- **#11 Placeholder shapes.** `looksLikeExample` recognises
  `<SET_ME>`, `<your-token>`, `__TOKEN__`, `${VAR}`, `xxxxx+`, and
  `....+` placeholders on the candidate's line.
- **#13 Binary file sniff.** `internal/walker.LooksBinary` reads the
  first 512 bytes of each file: NUL byte, invalid UTF-8, or more
  than 30 percent non-printable bytes classifies as binary and skips
  the file instead of feeding garbage into the regex/tokenizer.
- **#19 JSON safety on `Finding`.** `Finding.MarshalJSON` now omits
  `Span` by default, so `json.Marshal(finding)` cannot accidentally
  leak the raw secret value via library use. Callers that genuinely
  need the raw value (key rotation, the CLI
  `--output-reveal-secrets` flag) use the new `RevealedFinding`
  wrapper. `SafeForOutput` is now belt-and-suspenders for non-JSON
  sinks.

### Quality
Corpus thresholds (build gate) before / after on the labeled set
(now 28 cases, up from 22):

| Class   | v0.1.9 P/R    | v0.1.10 P/R   |
| ------- | ------------- | ------------- |
| secret  | 0.875 / 1.000 | 0.900 / 1.000 |
| pii     | 0.900 / 0.750 | 0.944 / 1.000 |

PII recall recovered to 1.0 thanks to the soft prefilter (#3),
spaced credit cards (#16), and looser phone shapes (#4).

### Notes
The fix for #18 (mask snap) reads `extractor.ActiveRules()` per
finding inside `MaskText`. This is fine for typical document sizes
but can be a hot path if a caller masks many findings on a very
large document. Future work: cache the per-document rule scan.

## [0.1.9] - 2026-04-26

### Security
- **Raw secret value (`span` field) is no longer in CLI output by
  default.** Previous releases emitted the raw secret in NDJSON, which
  meant any pipeline that piped findings to logs, dashboards, or CI
  artifacts was itself a leak — worse than not running hush at all.
  CLI now emits only `redacted` (first 3 + stars + last 3). Opt back
  in with `--output-reveal-secrets` for key-rotation pipelines that
  genuinely need the raw value, and only when the sink is private.
- **Library callers**: `Finding.Span` still holds the raw value for
  programmatic use (rotation, revocation). Use the new
  `scanner.SafeForOutput(findings)` helper before serializing to any
  external sink. Doc on `Finding` flags this explicitly.

## [0.1.8] - 2026-04-26

### Added
- **`hush detect` subcommand**: end-to-end NER pipeline (secrets and PII)
  using a new token-classification model embedded in the binary. Auto
  detects v2 vs v1 from the embedded asset. `--no-prefilter` opt-out
  for max-recall mode. `--model`/`--tokenizer` for a custom v2 hbin.
- **One-line v2 entry point** in the public `hush` package:
  `hush.New(hush.Options{UseDetector: true, DetectorPrefilter: true})`.
  No adapter, no `pkg/native` import required.
- **Hybrid regex + model fusion** for v2: regex finds candidates, model
  decides class and drops illustrative examples (READMEs, test
  fixtures). Catches everything regex catches plus contextual cases
  regex cannot describe (names in prose, custom internal tokens).
- **Regex+entropy prefilter** in front of the v2 model: clean files
  skip the model entirely. Bench: 4 KB clean doc 5.1s -> 5ms (1000x).
- **Embedded v2 model**: `hush-model-v2.int8.hbin` (79 MB, 7-class BIO
  over secret/pii/noise). v1 asset stays embedded for backward compat.
- **Labeled corpus** at `pkg/scanner/testdata/corpus/` with 22 files
  (secrets, PII, README examples, test fixtures, mixed) and a build
  gate at `TestCorpus_V1_vs_V2` enforcing v2 thresholds:
  secret P>=0.85 R>=0.80, pii P>=0.80 R>=0.70.

### Quality
Measured on the labeled corpus:

| Class   | v1 P/R       | v2 P/R       |
| ------- | ------------ | ------------ |
| secret  | 0.75 / 1.00  | 0.90 / 1.00  |
| pii     | 0.875 / 0.64 | 0.90 / 0.75  |

v2 strictly dominates v1 (more recall on PII, higher precision on both).

### Changed
- `examples/lib-basic` now demonstrates the v2 path.
- `examples/precommit` uses `hush detect`.

## [0.1.7] - 2026-04-25

### Fixed
- **PII findings no longer dropped by the classifier.** The shipped
  classifier was trained on credentials, so it scored real PII (emails,
  SSNs, credit cards) below threshold and silently filtered them out.
  Scanner now bypasses the model for PII rule candidates and reports
  them with regex confidence (1.0). `--detect=pii` finally returns what
  the regex actually matches.
- **`hush scan -` now reads stdin** (conventional Unix). Previously
  treated `-` as a path, scanned nothing, exited 0.
- **`hush.Default()` matches CLI defaults**: MinConfidence 0.5,
  CtxChars 256. Was 0.9 / 64 — too strict, caused confidence drift vs
  CLI on the same input.
- **`scanner.Options{}` zero-value CtxChars** bumped from 64 to 256 so
  library callers using zero-value Options see CLI-equivalent scores.

### Added
- **`(*scanner.Scanner).BatchScore([]SpanTriple)`** — library callers
  can batch-score without dropping into `pkg/native`. Falls back to
  per-candidate Score when the underlying scorer doesn't support batching.

## [0.1.6] - 2026-04-24

### Added
- **Batch scoring API**. Library users can now score many candidates in
  one call: `scanner.BatchScorer` interface, `SpanTriple` input type,
  `(*native.Scorer).BatchScore`, `(*native.Model).ForwardBatch`. Default
  `scanner.Scan` auto-dispatches when the scorer supports it.
- Numerical gate `TestForwardBatchMatchesForward` — batch output matches
  per-example Forward within 1e-3 at any batch size.

### Commentary
Benchmarks on M4 Pro show parity (~1.01×) vs looping Forward — the NEON
kernel already saturates per-example at realistic T. Shipped anyway as
infrastructure for: (a) CPUs with different overhead profiles where
batching will pay off, (b) future goroutine-parallel forwarding across
the batch dim, (c) consistent API for library consumers who want to
hand hush a slice of candidates. No regression on single-candidate
latency (within 1%).

## [0.1.5] - 2026-04-24

### Performance
- **NEON SIMD matmul kernel (arm64)**: hand-written 4×4 FMLA kernel
  replaces the pure-Go inner loop for the packed matmul.
  BenchmarkForward: **15.0 ms → 10.7 ms (-29%)**.
- **AVX2 / FMA kernel (amd64)**: XMM-width 4×4 kernel using
  VBROADCASTSS + VFMADD231PS. Cross-compiles clean; runtime exercised
  by release CI on linux/amd64.
- **Fallback**: pure-Go kernel remains for non-arm64/amd64 builds.
- Numerics unchanged (1e-4 vs ORT, 5e-2 int8 vs fp32). Allocs/op 76,
  memory/op ~13 KB — same as v0.1.4.

### Commentary
Tier 1 target was 5-7 ms. We hit ~11 ms. The remaining ~10 ms isn't
matmul anymore — it's split across embedding lookup, QKV projection,
softmax, layernorm, and GELU. Further speedups need attacking those
ops, not the matmul inner loop.

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
