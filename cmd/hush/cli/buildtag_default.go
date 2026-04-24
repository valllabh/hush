//go:build !native

package cli

// integrationBuildTags is the list of `-tags=...` values the integration
// TestMain passes to `go build`. Without the native tag we use the
// default (ORT) backend; the string stays empty.
var integrationBuildTags = ""
