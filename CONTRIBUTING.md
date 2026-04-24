# Contributing to hush

Thanks for helping. Contributions of any size are welcome.

## Dev setup

```
git clone https://github.com/valllabh/hush.git
cd hush
go build ./cmd/hush
go test ./...
```

Go 1.26 or newer. ONNX Runtime shared library must be present for
tests that exercise the classifier (`brew install onnxruntime` on
macOS, or download from https://github.com/microsoft/onnxruntime/releases).

### Build backends

Hush ships two inference backends, selected at build time via Go build tags:

- Default (no tag): ORT backend via `pkg/classifier`. Requires CGO and the
  ONNX Runtime shared library at runtime. Fastest today.

  ```
  go build ./cmd/hush
  ```

- `native` tag: pure-Go runtime via `pkg/native`. No CGO, no libonnxruntime
  in the shipped binary. Slower end-to-end but trivially portable.

  ```
  go build -tags=native ./cmd/hush
  go test  -tags=native ./...
  ```

The switch is wired in `pkg/bundled/bundled_ort.go` (tag `!native`) and
`pkg/bundled/bundled_native.go` (tag `native`), both of which register
`scanner.DefaultScorerFactory`.

## Before you open a PR

1. `go test ./...` passes.
2. `go vet ./...` clean.
3. `gofmt -s -w .` applied.
4. New public API has doc comments.
5. Behaviour changes ship with tests.
6. If the change affects performance (kernel, hot path, binary size,
   startup, RSS), update [PERF.md](PERF.md) with the new numbers and a
   dated row in the relevant progression table. Regressions need a
   written reason.

## Commit messages

Use conventional commits. Examples:

```
feat(scanner): add sarif output format
fix(extractor): handle utf16 byte order marks
docs(readme): clarify model download flow
```

## Releasing

Maintainers only:

```
git tag v0.2.0 -m "hush v0.2.0"
git push origin v0.2.0
```

The release workflow builds native binaries on a matrix (linux/amd64,
linux/arm64, darwin/amd64, darwin/arm64), bundles the matching
libonnxruntime into each archive, generates SHA256SUMS, signs every
archive with cosign (keyless via GitHub OIDC), attests build provenance,
and publishes everything as a GitHub Release.

## Scope

Hush stays focused on secrets detection. PRs that expand scope to
generic linting, license scanning, or policy enforcement will be
redirected to a separate tool.

## Code of Conduct

All contributors agree to follow the [Code of Conduct](CODE_OF_CONDUCT.md).
