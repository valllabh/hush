//go:build native

package cli

// integrationBuildTags tells the integration TestMain to compile hush
// with the same native tag that this test binary was built with, so the
// exec'd binary exercises the pure Go backend rather than the default
// ORT path.
var integrationBuildTags = "native"
