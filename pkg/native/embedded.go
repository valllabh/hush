package native

import (
	"bytes"
	_ "embed"
)

// ModelVersion is the identifier of the embedded classifier model.
// Keep in sync with the asset filename and CHANGELOG.
const ModelVersion = "hush-model-v1"

//go:embed assets/hush-model-v1.int8.hbin
var embeddedModel []byte

//go:embed assets/hush-model-v1.tokenizer.json
var embeddedTokenizer []byte

// NewBundledScorer constructs a Scorer from the int8 model and tokenizer
// embedded in the binary via go:embed. No filesystem access is required.
func NewBundledScorer() (*Scorer, error) {
	return LoadScorerReader(bytes.NewReader(embeddedModel), bytes.NewReader(embeddedTokenizer))
}
