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

## Before you open a PR

1. `go test ./...` passes.
2. `go vet ./...` clean.
3. `gofmt -s -w .` applied.
4. New public API has doc comments.
5. Behaviour changes ship with tests.

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
