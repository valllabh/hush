<p align="center">
  <img src="docs/assets/logo.png" alt="hush" width="180" />
</p>

<h1 align="center">hush</h1>

<p align="center">
  <b>AI based Secrets Detector. Runs on CPU.</b><br/>
  A tiny AI that catches real leaks regex scanners miss,<br/>
  and ignores the noise they flood you with. Offline. No GPU. No cloud.
</p>

<p align="center">

[![ci](https://github.com/valllabh/hush/actions/workflows/ci.yml/badge.svg)](https://github.com/valllabh/hush/actions/workflows/ci.yml)
[![Go Reference](https://pkg.go.dev/badge/github.com/valllabh/hush.svg)](https://pkg.go.dev/github.com/valllabh/hush)
[![License: Apache-2.0](https://img.shields.io/badge/License-Apache--2.0-blue.svg)](LICENSE)
[![Release](https://img.shields.io/github/v/release/valllabh/hush)](https://github.com/valllabh/hush/releases)

</p>

## What is it

Hush scans files for secrets — API keys, tokens, passwords, certificates — and flags them before they leak.

What makes it different: there's a little AI brain inside the binary that double-checks every suspicious-looking string. Regex scanners yell about every random ID or hash; hush only flags the ones the AI thinks are actually secrets. Fewer false alarms, real leaks don't get drowned out.

- **One file to run.** A single static binary. No Python, no libraries to install, no Docker required.
- **Completely offline.** Your code never leaves your machine.
- **No GPU needed.** Runs on any CPU — your laptop, your CI, a Raspberry Pi.
- **Fast.** Thousands of files per second.
- **Works everywhere.** Use it as a CLI, in CI, as a Go library, in a pre-commit hook, or wherever you move text around.

## Install

Pick one:

```
# Download prebuilt binary (Linux or macOS)
curl -sSL https://github.com/valllabh/hush/releases/latest/download/hush_0.1.2_linux_amd64.tar.gz | tar xz
sudo mv hush_0.1.2_linux_amd64/hush /usr/local/bin/

# Or with Go
go install github.com/valllabh/hush/cmd/hush@latest

# Or in Docker
docker run --rm -v "$PWD:/src:ro" ghcr.io/valllabh/hush:latest scan /src
```

That's it. No additional setup.

## Use it

```
$ hush scan .
path/config.py:12:18  aws_access_key   AKIA****************  0.98
path/.env:3:14        slack_token      xoxb-****             0.96

2 findings in 143 files (0.42s)
```

Exit code is nonzero when secrets are found, so it drops straight into CI.

### Common patterns

```
# Scan a directory
hush scan ./my-repo

# Pipe anything into it
cat suspicious.log | hush scan -

# Get JSON (for scripts or CI)
hush scan . --output-json > findings.json

# Get SARIF (for GitHub Code Scanning)
hush scan . --format sarif > hush.sarif

# Fail the build on any finding
hush scan . --fail-on-finding

# Skip irrelevant paths
hush scan . --file-exclude 'node_modules,vendor,*.min.js'

# Only very confident findings
hush scan . --model-threshold 0.95
```

Run `hush --help` for the full list. Config can also live in a `.hush.yaml` file at your repo root.

## As a Go library

```go
import "github.com/valllabh/hush"

s, _ := hush.New(hush.Options{MinConfidence: 0.9})
defer s.Close()

findings, _ := s.ScanReader(reader)
masked, _, _ := s.Redact(text, "[REDACTED:%s]")
```

One import, one call, done. See [`examples/lib-basic`](examples/lib-basic/) for a runnable version.

## Where to plug it in

Twenty plus drop-in patterns in [`examples/`](examples/). Highlights:

**Catch secrets before they ship**
- [pre-commit hook](examples/precommit/) &middot; [GitHub Actions + SARIF](examples/github-actions/) &middot; [GitLab CI](examples/gitlab-ci/) &middot; [Docker image scan](examples/docker-scan/)

**Mask secrets in motion**
- [text redaction](examples/lib-data-masking/) &middot; [LLM prompt proxy](examples/lib-llm-proxy/) &middot; [Kafka stream filter](examples/lib-kafka-filter/) &middot; [log redaction](examples/lib-logging-hook/) &middot; [Slack warning bot](examples/lib-slack-bot/)

**Guard your pipelines**
- [RAG ingest scanner](examples/lib-rag-ingest/) &middot; [S3 backup sweep](examples/backup-sweep/) &middot; [Kubernetes admission](examples/k8s-admission/) &middot; [shell history audit](examples/shell-history/)

**Developer experience**
- [editor LSP](examples/ide-lsp/) &middot; [browser extension](examples/browser-extension/) &middot; [REST sidecar](examples/rest-server/) &middot; [gRPC service](examples/grpc-service/)

## How it compares

| tool        | offline | AI model | runtime deps | single binary |
| ----------- | ------- | -------- | ------------ | ------------- |
| **hush**    | yes     | yes      | **none**     | **yes**       |
| trufflehog  | yes     | partial  | none         | yes           |
| gitleaks    | yes     | no       | none         | yes           |
| ggshield    | no      | yes      | Python + API | no            |

Hush favours precision over recall. For compliance-style exhaustive coverage, pair hush with gitleaks.

## Verify a release

Every release archive is signed with [cosign](https://github.com/sigstore/cosign) via GitHub Actions OIDC. Verify it hasn't been tampered with:

```
cosign verify-blob \
  --certificate hush_0.1.2_linux_amd64.tar.gz.pem \
  --signature   hush_0.1.2_linux_amd64.tar.gz.sig \
  --certificate-identity-regexp "https://github.com/valllabh/hush/.github/workflows/release.yml.*" \
  --certificate-oidc-issuer https://token.actions.githubusercontent.com \
  hush_0.1.2_linux_amd64.tar.gz
```

## How it works (simple version)

Two steps:

1. **Find suspects.** Regex rules + entropy checks pull out every string that *could* be a secret. This is fast but noisy — it catches real secrets along with a lot of harmless-looking garbage.
2. **Ask the AI.** A tiny AI model looks at each suspect and its surroundings and decides: real secret, or just noise? Only the confident ones get reported.

The AI is trained on thousands of real leaks and near-miss examples, so it's good at telling an AWS key apart from a UUID that happens to look similar.

It all runs on your CPU. No GPU needed. No cloud involved. The model is baked right into the binary.

Performance numbers and technical detail: [PERF.md](PERF.md) and [CHANGELOG.md](CHANGELOG.md).

## Security · Contributing · License

- Security policy and vulnerability reporting: [SECURITY.md](SECURITY.md)
- Dev setup and contribution guide: [CONTRIBUTING.md](CONTRIBUTING.md)
- Code of conduct: [CODE_OF_CONDUCT.md](CODE_OF_CONDUCT.md)
- License: [Apache 2.0](LICENSE)
