package native

import (
	"bytes"
	_ "embed"
)

//go:embed assets/hush-model-v1.int8.hbin
var embeddedModel []byte

//go:embed assets/hush-model-v1.tokenizer.json
var embeddedTokenizer []byte

// NewBundledScorer constructs a Scorer from the int8 model and tokenizer
// embedded in the binary via go:embed. No filesystem access is required.
func NewBundledScorer() (*Scorer, error) {
	return LoadScorerReader(bytes.NewReader(embeddedModel), bytes.NewReader(embeddedTokenizer))
}
