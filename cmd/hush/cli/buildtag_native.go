//go:build ort

package cli

// integrationBuildTags propagates the `ort` tag to `go build` when the
// test binary was built with it, so the exec'd hush exercises the ORT
// backend for diff testing rather than the default pure-Go path.
var integrationBuildTags = "ort"
