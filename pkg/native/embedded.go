package native

import (
	"bytes"
	_ "embed"
)

// ModelVersion is the identifier of the embedded v1 sequence-classification
// model. Keep in sync with the asset filename and CHANGELOG.
const ModelVersion = "hush-model-v1"

// ModelVersionV2 identifies the embedded v2 token-classification (NER) model.
const ModelVersionV2 = "hush-model-v2"

//go:embed assets/hush-model-v1.int8.hbin
var embeddedModel []byte

//go:embed assets/hush-model-v1.tokenizer.json
var embeddedTokenizer []byte

//go:embed assets/hush-model-v2.int8.hbin
var embeddedModelV2 []byte

//go:embed assets/hush-model-v2.tokenizer.json
var embeddedTokenizerV2 []byte

// NewBundledScorer constructs a Scorer from the int8 v1 model and tokenizer
// embedded in the binary via go:embed. No filesystem access is required.
func NewBundledScorer() (*Scorer, error) {
	return LoadScorerReader(bytes.NewReader(embeddedModel), bytes.NewReader(embeddedTokenizer))
}

// NewBundledDetector constructs a Detector from the int8 v2 NER model and
// tokenizer embedded via go:embed. No filesystem access is required.
func NewBundledDetector() (*Detector, error) {
	return LoadDetectorReader(bytes.NewReader(embeddedModelV2), bytes.NewReader(embeddedTokenizerV2))
}

// EmbeddedV2Meta reads the embedded v2 hbin and returns just its Meta. The
// hbin format requires sequential reads, so this loads tensors too; the
// bundle is dropped immediately and Meta is a small struct, so the cost is
// transient. Use this from the CLI to auto-detect Task without committing
// to a full Detector load.
func EmbeddedV2Meta() (*Meta, error) {
	b, err := Read(bytes.NewReader(embeddedModelV2))
	if err != nil {
		return nil, err
	}
	m := b.Meta
	return &m, nil
}
