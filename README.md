<p align="center">
  <img src="docs/assets/logo.png" alt="hush" width="180" />
</p>

<h1 align="center">hush</h1>

<p align="center">
  <b>Find secrets and personal data in your code and files. Runs locally on any laptop.</b><br/>
  One small binary. No cloud. No GPU. No setup.
</p>

<p align="center">

[![ci](https://github.com/valllabh/hush/actions/workflows/ci.yml/badge.svg)](https://github.com/valllabh/hush/actions/workflows/ci.yml)
[![Go Reference](https://pkg.go.dev/badge/github.com/valllabh/hush.svg)](https://pkg.go.dev/github.com/valllabh/hush)
[![License: Apache-2.0](https://img.shields.io/badge/License-Apache--2.0-blue.svg)](LICENSE)
[![Release](https://img.shields.io/github/v/release/valllabh/hush)](https://github.com/valllabh/hush/releases)

</p>

## What it does

Hush reads files and points out two kinds of things you usually do not want sitting in your code or logs:

- **Secrets** — API keys, tokens, passwords, certificates.
- **Personal data (PII)** — emails, phone numbers, names, addresses, social security numbers, credit card numbers.

It works by combining two checks:

1. A list of patterns that match common secrets and personal data.
2. A small built in AI that reads the surrounding text and decides if a match is real, or just an example in a README, or something harmless that happens to look suspicious.

You get the things pattern scanners catch, plus the ones they cannot describe with a pattern (like a person's name in a sentence), and far fewer false alarms.

## Why it is different

- **One file to run.** A single binary. No Python, no libraries, no Docker.
- **Works offline.** Your code never leaves your machine.
- **No GPU.** Runs on any laptop or CI machine.
- **Fast on clean files.** If a file has nothing suspicious, hush moves on in milliseconds.
- **Use it anywhere.** Command line, CI, Go library, pre commit hook, log filter.

## Install

```
# Download a prebuilt binary (Linux or macOS)
curl -sSL https://github.com/valllabh/hush/releases/latest/download/hush_linux_amd64.tar.gz | tar xz
sudo mv hush /usr/local/bin/

# Or with Go
go install github.com/valllabh/hush/cmd/hush@latest
```

## Use it

```
$ hush detect .
config/prod.yaml:12:18  secret  AKIA****************            0.99
.env:3:14               secret  ghp_***                         0.99
docs/onboard.md:7:9     pii     val*****************com         0.99
docs/onboard.md:9:14    pii     +1***********67                 0.97

4 findings in 143 files (0.4s)
```

Exit code is non zero when findings are reported, so it drops straight into CI.

### Common patterns

```
# Scan a directory
hush detect ./my-repo

# Pipe text in
cat suspicious.log | hush detect

# Save findings as JSON
hush detect . > findings.jsonl

# Plug your own model in
hush detect --model my-model.hbin --tokenizer my-tokenizer.json ./my-repo
```

Run `hush --help` for the full list.

## As a Go library

```go
import "github.com/valllabh/hush"

s, _ := hush.New(hush.Options{
    UseDetector:       true, // catches secrets and PII
    DetectorPrefilter: true, // fast on clean files
    MinConfidence:     0.5,
})
defer s.Close()

findings, _ := s.ScanReader(reader)
masked, _, _ := s.Redact(text, "[REDACTED:%s]")
```

One import. One call. Done. See [`examples/lib-basic`](examples/lib-basic/) for a runnable version.

## Where to plug it in

Drop in patterns in [`examples/`](examples/):

- [pre commit hook](examples/precommit/), [GitHub Actions](examples/github-actions/), [GitLab CI](examples/gitlab-ci/), [Docker image scan](examples/docker-scan/)
- [text redaction](examples/lib-data-masking/), [LLM prompt proxy](examples/lib-llm-proxy/), [Kafka stream filter](examples/lib-kafka-filter/), [log redaction](examples/lib-logging-hook/)
- [RAG ingest scanner](examples/lib-rag-ingest/), [S3 backup sweep](examples/backup-sweep/), [Kubernetes admission](examples/k8s-admission/)
- [editor LSP](examples/ide-lsp/), [browser extension](examples/browser-extension/), [REST sidecar](examples/rest-server/), [gRPC service](examples/grpc-service/)

## Quality

Tested against a small labeled set of 22 files (real secrets, real PII, README examples, test fixtures, mixed):

| Category   | Caught | False alarms |
| ---------- | ------ | ------------ |
| secrets    | 100%   | 1 in 10      |
| PII        | 75%    | 1 in 10      |

Plain English: every secret in the test set was caught. About one in ten flagged items is a false alarm. Personal data coverage includes things older tools miss like names in prose. The test set lives in [`pkg/scanner/testdata/corpus`](pkg/scanner/testdata/corpus/), and the build fails if these numbers regress.

## How it compares

| tool        | offline | uses AI | covers PII | extra setup       | single binary |
| ----------- | ------- | ------- | ---------- | ----------------- | ------------- |
| **hush**    | yes     | yes     | **yes**    | **none**          | **yes**       |
| trufflehog  | yes     | partial | narrow     | none              | yes           |
| gitleaks    | yes     | no      | narrow     | none              | yes           |
| ggshield    | no      | yes     | partial    | Python plus API   | no            |

For exhaustive secret only coverage with no AI, pair hush with gitleaks.

## Verify a release

Every release archive is signed via GitHub Actions. Verify with [cosign](https://github.com/sigstore/cosign):

```
cosign verify-blob \
  --certificate hush_linux_amd64.tar.gz.pem \
  --signature   hush_linux_amd64.tar.gz.sig \
  --certificate-identity-regexp "https://github.com/valllabh/hush/.github/workflows/release.yml.*" \
  --certificate-oidc-issuer https://token.actions.githubusercontent.com \
  hush_linux_amd64.tar.gz
```

## How it works

1. **Look for anything that could be a secret or personal data.** A list of about 100 patterns plus a randomness check pulls out candidates: AWS keys, GitHub tokens, emails, phone numbers, credit cards, addresses, and so on. Cheap and fast. Files with no candidates skip the next step.

2. **Have the AI take a look.** For each candidate, the AI reads the words around it and decides:
   - Real. Keep it.
   - Just an example in a README or test file. Drop it.
   - Bonus: sometimes the AI spots names, internal tokens, or addresses that no pattern describes. Those become extra findings.

The AI has been trained on labeled examples so it can tell a real AWS key from one in a tutorial that says "do not use". The model is baked into the binary and runs on your CPU.

Numbers and benchmarks: [PERF.md](PERF.md) and [CHANGELOG.md](CHANGELOG.md).

## Security · Contributing · License

- Security policy and vulnerability reporting: [SECURITY.md](SECURITY.md)
- Dev setup and contribution guide: [CONTRIBUTING.md](CONTRIBUTING.md)
- Code of conduct: [CODE_OF_CONDUCT.md](CODE_OF_CONDUCT.md)
- License: [Apache 2.0](LICENSE)
