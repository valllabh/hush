# Examples

Patterns for using hush across CI, data pipelines, services, and
developer tools. Every folder is a short README with a drop in snippet.
One folder, [`lib-basic`](lib-basic/), contains a complete runnable
Go program you can start from.

## Library

- [lib-basic](lib-basic/) runnable: minimal scanner
- [lib-data-masking](lib-data-masking/) redact secrets in strings
- [lib-llm-proxy](lib-llm-proxy/) strip secrets from outgoing LLM prompts
- [lib-kafka-filter](lib-kafka-filter/) pre ingest Kafka filter
- [lib-rag-ingest](lib-rag-ingest/) scan docs before vector DB
- [lib-slack-bot](lib-slack-bot/) catch pastes in public Slack channels
- [lib-email-gateway](lib-email-gateway/) outbound SMTP redactor
- [lib-logging-hook](lib-logging-hook/) slog / zap redaction
- [lib-clipboard-watch](lib-clipboard-watch/) desktop clipboard monitor

## CLI in CI

- [precommit](precommit/) local git pre commit hook
- [github-actions](github-actions/) PR scan + SARIF upload
- [gitlab-ci](gitlab-ci/) MR scan + SAST report
- [docker-scan](docker-scan/) scan image layers
- [ci-blame-bot](ci-blame-bot/) auto assign leaks to authors

## Sweeps and monitors

- [shell-history](shell-history/) scan bash / zsh history
- [log-tail](log-tail/) tail app logs through hush
- [backup-sweep](backup-sweep/) scan S3 dumps via Lambda
- [k8s-admission](k8s-admission/) block risky ConfigMaps and Secrets

## Services

- [rest-server](rest-server/) HTTP sidecar
- [grpc-service](grpc-service/) gRPC service

## Developer experience

- [ide-lsp](ide-lsp/) diagnostics via LSP
- [browser-extension](browser-extension/) WASM build scanning web forms
- [obsidian-plugin](obsidian-plugin/) scan your personal notes vault

## Contributing examples

Got an idea? Open a PR. Keep it short: one README, one snippet. Working
code is welcome but not required.
