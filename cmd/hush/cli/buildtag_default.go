//go:build !ort

package cli

// integrationBuildTags is the list of `-tags=...` values the integration
// TestMain passes to `go build`. The default build is the pure-Go path;
// no extra tags needed.
var integrationBuildTags = ""
