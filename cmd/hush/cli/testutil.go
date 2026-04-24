package cli

import "github.com/valllabh/hush/pkg/extractor"

// getDefaultRulesJSON exposes the embedded rules JSON to tests in this
// package without adding a new export to the extractor package.
func getDefaultRulesJSON() []byte { return extractor.DefaultRulesJSON() }
