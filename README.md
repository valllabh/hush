<p align="center">
  <img src="docs/assets/logo.png" alt="hush" width="180" />
</p>

<h1 align="center">hush</h1>

<p align="center">
  <b>AI based Secrets Detector. Runs on CPU.</b><br/>
  An 80MB binary with a brain. Catches real leaks that regex scanners miss,<br/>
  ignores the noise they flood you with. Offline. No GPU. No cloud.
</p>

<p align="center">

[![ci](https://github.com/valllabh/hush/actions/workflows/ci.yml/badge.svg)](https://github.com/valllabh/hush/actions/workflows/ci.yml)
[![Go Reference](https://pkg.go.dev/badge/github.com/valllabh/hush.svg)](https://pkg.go.dev/github.com/valllabh/hush)
[![Go Report Card](https://goreportcard.com/badge/github.com/valllabh/hush)](https://goreportcard.com/report/github.com/valllabh/hush)
[![License: Apache-2.0](https://img.shields.io/badge/License-Apache--2.0-blue.svg)](LICENSE)
[![Release](https://img.shields.io/github/v/release/valllabh/hush)](https://github.com/valllabh/hush/releases)

</p>

## Why hush

Every secrets scanner is regex plus entropy. They catch secrets, and they
catch everything that *looks* like one: UUIDs, commit hashes, base64 blobs,
minified bundles. Your engineers grow numb, real leaks slip past.

Hush adds a brain. A 1.58 bit quantized classifier, trained on real and
synthetic credentials, decides whether a candidate is an actual secret or
just noise. The whole model fits inside the binary. Scanning an entire
repo still finishes in seconds.

- **smarter** — learned classifier cuts false positives vs regex alone
- **smaller** — 80MB binary with the model baked in
- **portable** — one static binary, any CPU, no GPU
- **private** — no cloud calls, no telemetry, your bytes stay on your box
- **everywhere** — CLI, Go library, pre commit hook, CI, log pipeline, LLM proxy

## Install

```
# macOS
brew install valllabh/tap/hush

# go install
go install github.com/valllabh/hush/cmd/hush@latest

# prebuilt binaries (bundle libonnxruntime, just extract and run)
curl -sSL https://github.com/valllabh/hush/releases/latest/download/hush_0.1.0_linux_amd64.tar.gz | tar xz

# docker
docker run --rm -v "$PWD:/src:ro" ghcr.io/valllabh/hush:latest scan /src
```

Install also needs ONNX Runtime available on the system. Hush will use the
system `libonnxruntime` at runtime.

## Quick start

```
$ hush scan path/to/repo
path/to/repo/config.py:12:18  aws_access_key  AKIA****************  0.98
path/to/repo/.env:3:14        slack_token     xoxb-****             0.96

2 findings in 143 files (0.42s)
```

Exit code is nonzero when findings are reported, so hush drops straight into CI.

## Usage

### CLI

```
hush scan <path>              scan a file or directory
hush scan -                   scan stdin
hush rules list               list detection rules
hush version                  print version and embedded model version
```

Common flags:

```
--format {text|json|sarif}   output format (default text)
--min-confidence 0.90        drop findings below this score
--ignore PATTERN             glob pattern to skip (repeatable)
--workers N                  parallel workers (default NumCPU)
--model PATH                 override the embedded model
--threads N                  ONNX Runtime intra op threads
--fail-on-finding            exit nonzero if any finding
```

### Library

One import, product namespace:

```go
import "github.com/valllabh/hush"

s, err := hush.New(hush.Options{MinConfidence: 0.9})
if err != nil { panic(err) }
defer s.Close()

findings, err := s.ScanReader(strings.NewReader(text))
for _, f := range findings {
    fmt.Printf("%s %s confidence=%.2f\n", f.Rule, f.Redacted, f.Confidence)
}

// redact inline
masked, findings, _ := s.Redact(text, "[REDACTED:%s]")
fmt.Println(hush.ModelVersion) // "v1"
```

See [`examples/lib-basic`](examples/lib-basic/) for a runnable version.

#### Advanced

Power users can reach the underlying packages to swap the classifier,
drive extraction directly, or provide a custom model:

```go
import (
    "github.com/valllabh/hush/pkg/scanner"
    "github.com/valllabh/hush/pkg/extractor"
)
// bring your own scorer, custom extractor rules, etc.
```

The `hush.*` API should cover 95% of use cases; reach for `pkg/*`
when you really need to.

## Configuration

Config can live in `.hush.yaml` at the repo root, `$HOME/.config/hush/config.yaml`,
or be passed with `--config path.yaml`.

```yaml
min_confidence: 0.9
ignore:
  - "**/vendor/**"
  - "**/*.min.js"
workers: 8
rules:
  - aws_access_key
  - slack_token
  - stripe_key
  - generic_api_key
```

All config keys can be overridden via env vars prefixed `HUSH_` (e.g.
`HUSH_MIN_CONFIDENCE=0.95`).

## Output formats

### JSON

```json
{
  "path": "config.py",
  "line": 12,
  "column": 18,
  "rule": "aws_access_key",
  "match": "AKIA****************",
  "score": 0.98
}
```

### SARIF

Emit SARIF 2.1.0 for GitHub Code Scanning:

```
hush scan --format sarif . > hush.sarif
```

Wire it up with [`github/codeql-action/upload-sarif`](https://github.com/github/codeql-action).

## Performance

Measured on a MacBook Pro M2, scanning the Samsung CredData repository
(1.2M lines across 8k files).

| metric                | value          |
| --------------------- | -------------- |
| throughput            | 4.8k files/sec |
| classifier latency p50| 0.8 ms         |
| classifier latency p99| 3.1 ms         |
| RSS memory            | 180 MB         |
| binary size           | 83 MB          |

See [`hush bench`](cmd/hush/) for reproducible benchmarks.

## Model

Hush embeds a BitNet b1.58 classifier, fine tuned on a mix of
Samsung CredData, NVIDIA Nemotron PII, and synthetic data. The model
is quantized to 1.58 bits, yielding sub millisecond classification
on CPU.

- current version: `hush-model-v1`
- released with: hush v0.1.0
- override at runtime: `hush scan --model ./my-model.onnx`

Read the embedded model version programmatically from Go:

```go
import "github.com/valllabh/hush/pkg/classifier"
fmt.Println(classifier.ModelVersion) // "v1"
```

## Accuracy

Held out evaluation on CredData (83 repos, 23k findings, 1.2M lines).

| rule family       | precision | recall | F1   |
| ----------------- | --------- | ------ | ---- |
| AWS keys          | 0.99      | 0.96   | 0.97 |
| Slack tokens      | 0.98      | 0.94   | 0.96 |
| Stripe keys       | 0.99      | 0.97   | 0.98 |
| Generic API keys  | 0.87      | 0.78   | 0.82 |
| Private keys      | 1.00      | 0.99   | 0.99 |
| overall           | 0.94      | 0.89   | 0.91 |

Numbers refresh on each model release, tracked in [CHANGELOG.md](CHANGELOG.md).

## How it works

```
text -> candidate extractor -> classifier -> verdict
         (regex + entropy)    (BitNet ONNX)
```

1. Extractor finds candidate spans using regex rules and Shannon entropy.
2. Each candidate plus context is scored by the embedded classifier.
3. Candidates above the threshold are reported.

The two stage design means expensive classification only runs on
candidates, keeping throughput high while false positives stay low.

## Comparison

| tool        | offline | learned | binary size | typical FP rate |
| ----------- | ------- | ------- | ----------- | --------------- |
| hush        | yes     | yes     | 83 MB       | low             |
| trufflehog  | yes     | partial | 62 MB       | medium          |
| gitleaks    | yes     | no      | 12 MB       | high            |
| ggshield    | no      | yes     | pip         | low (cloud)     |

Hush favours precision over recall on purpose. For compliance style
exhaustive coverage, pair hush with gitleaks.

## Examples

Twenty plus patterns for using hush, grouped by goal. Each folder contains
a README with drop in snippets. One folder, [`lib-basic`](examples/lib-basic),
contains a complete runnable Go program.

### Catch secrets before they ship
- [pre commit hook](examples/precommit/)
- [GitHub Actions with SARIF upload](examples/github-actions/)
- [GitLab CI](examples/gitlab-ci/)
- [Docker image layer scan](examples/docker-scan/)
- [CI blame bot](examples/ci-blame-bot/)

### Mask secrets in flight
- [library: basic scan](examples/lib-basic/)
- [library: text redaction](examples/lib-data-masking/)
- [library: LLM prompt proxy](examples/lib-llm-proxy/)
- [library: Kafka stream filter](examples/lib-kafka-filter/)
- [library: slog log redaction](examples/lib-logging-hook/)
- [library: email SMTP gateway](examples/lib-email-gateway/)
- [library: Slack warning bot](examples/lib-slack-bot/)
- [library: clipboard watcher](examples/lib-clipboard-watch/)

### Guard ingestion pipelines
- [library: RAG document scanner](examples/lib-rag-ingest/)
- [backup sweep on S3](examples/backup-sweep/)
- [Kubernetes admission controller](examples/k8s-admission/)
- [shell history sweep](examples/shell-history/)
- [log tail filter](examples/log-tail/)

### Developer experience
- [LSP server for editors](examples/ide-lsp/)
- [browser extension (WASM)](examples/browser-extension/)
- [Obsidian plugin](examples/obsidian-plugin/)

### Services
- [REST sidecar](examples/rest-server/)
- [gRPC service](examples/grpc-service/)

## Security

Hush handles sensitive inputs by design. See [SECURITY.md](SECURITY.md) for the
threat model and how to report vulnerabilities privately.

Binaries are signed with [cosign](https://github.com/sigstore/cosign) via
the GitHub Actions OIDC identity, and every release ships a SLSA build
provenance attestation. Verify a release:

```
cosign verify-blob \
  --certificate hush_0.1.0_linux_amd64.tar.gz.pem \
  --signature   hush_0.1.0_linux_amd64.tar.gz.sig \
  --certificate-identity-regexp "https://github.com/valllabh/hush/.github/workflows/release.yml.*" \
  --certificate-oidc-issuer https://token.actions.githubusercontent.com \
  hush_0.1.0_linux_amd64.tar.gz
```

## Contributing

Patches welcome. See [CONTRIBUTING.md](CONTRIBUTING.md) for dev setup,
testing, and commit conventions. By participating you agree to follow
the [Code of Conduct](CODE_OF_CONDUCT.md).

## License

[Apache 2.0](LICENSE).

## Citation

If you use hush in research, please cite:

```bibtex
@software{hush,
  author  = {Vallabh Joshi},
  title   = {hush: 1 bit quantized secrets detection},
  year    = {2026},
  url     = {https://github.com/valllabh/hush}
}
```
