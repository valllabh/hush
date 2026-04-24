package native

import (
	"bytes"
	"fmt"
	"io"
	"math"
	"os"

	"github.com/sugarme/tokenizer"
	"github.com/sugarme/tokenizer/pretrained"
	"github.com/valllabh/hush/pkg/scanner"
)

// Scorer wraps a loaded Model plus its tokenizer and presents the
// scanner.Scorer contract used by the rest of hush. It is the single
// integration point for the pure Go runtime.
//
// Construct one via NewScorer when you already have a loaded *Model,
// or via LoadScorer which reads model + tokenizer from paths. Safe
// for concurrent use only when the underlying Model is — the current
// implementation is not (it mutates tensor buffers during Forward).
// Guard with a sync.Mutex or one Scorer per goroutine if you need
// parallelism.
type Scorer struct {
	model *Model
	tk    *tokenizer.Tokenizer

	// maxLen defines the upper bound on tokenized sequence length.
	// Defaults to the model's SeqLen (typically 384 or 256). The
	// Forward pass dynamically trims trailing pad tokens so shorter
	// inputs cost less.
	maxLen int
}

// NewScorer returns a Scorer using an already-loaded model and tokenizer.
// maxLen of <= 0 falls back to model.Meta.SeqLen.
func NewScorer(model *Model, tk *tokenizer.Tokenizer, maxLen int) *Scorer {
	if maxLen <= 0 {
		maxLen = model.Meta.SeqLen
	}
	return &Scorer{model: model, tk: tk, maxLen: maxLen}
}

// LoadScorer reads a Model from modelPath (.hbin) and a tokenizer from
// tokenizerPath (HuggingFace tokenizer.json) and returns a ready Scorer.
func LoadScorer(modelPath, tokenizerPath string) (*Scorer, error) {
	f, err := os.Open(modelPath)
	if err != nil {
		return nil, fmt.Errorf("open model: %w", err)
	}
	defer f.Close()
	b, err := Read(f)
	if err != nil {
		return nil, fmt.Errorf("read model: %w", err)
	}
	m, err := LoadModel(b)
	if err != nil {
		return nil, fmt.Errorf("load model: %w", err)
	}

	tkData, err := os.ReadFile(tokenizerPath)
	if err != nil {
		return nil, fmt.Errorf("read tokenizer: %w", err)
	}
	tk, err := pretrained.FromReader(bytes.NewReader(tkData))
	if err != nil {
		return nil, fmt.Errorf("parse tokenizer: %w", err)
	}
	return NewScorer(m, tk, 0), nil
}

// LoadScorerReader is like LoadScorer but takes io.Reader sources so
// callers can feed embedded bytes or any other stream.
func LoadScorerReader(modelR io.Reader, tokenizerR io.Reader) (*Scorer, error) {
	b, err := Read(modelR)
	if err != nil {
		return nil, fmt.Errorf("read model: %w", err)
	}
	m, err := LoadModel(b)
	if err != nil {
		return nil, fmt.Errorf("load model: %w", err)
	}
	tk, err := pretrained.FromReader(tokenizerR)
	if err != nil {
		return nil, fmt.Errorf("parse tokenizer: %w", err)
	}
	return NewScorer(m, tk, 0), nil
}

// Close is a no-op today (no CGO resources to release), present so the
// Scorer satisfies the same contract as pkg/classifier.Classifier.
func (s *Scorer) Close() error { return nil }

// ModelVersion returns the embedded model version from the bundle's meta.
// The native runtime doesn't hard-code a version the way pkg/classifier
// does, so we expose the one the user loaded.
func (s *Scorer) ModelVersion() string {
	return s.model.Meta.Model
}

// Score returns the probability [0,1] that span is a secret, given its
// surrounding left/right context. Matches pkg/classifier.Classifier.Score
// numerically (to within classifier fp precision).
func (s *Scorer) Score(left, span, right string) (float64, error) {
	text := left + "[CAND]" + span + "[/CAND]" + right
	enc, err := s.tk.EncodeSingle(text, true)
	if err != nil {
		return 0, fmt.Errorf("encode: %w", err)
	}
	ids := enc.Ids
	mask := enc.AttentionMask

	if len(ids) > s.maxLen {
		ids = ids[:s.maxLen]
		mask = mask[:s.maxLen]
	}
	// Pad to maxLen with RoBERTa pad_token_id=1 so attention_mask alignment
	// stays consistent; Forward trims this before running.
	for len(ids) < s.maxLen {
		ids = append(ids, 1)
		mask = append(mask, 0)
	}

	ids32 := make([]int32, s.maxLen)
	mask32 := make([]int32, s.maxLen)
	for i := 0; i < s.maxLen; i++ {
		ids32[i] = int32(ids[i])
		mask32[i] = int32(mask[i])
	}

	logits := s.model.Forward(ids32, mask32)
	if len(logits) != 2 {
		return 0, fmt.Errorf("expected 2 logits, got %d", len(logits))
	}
	return softmax2(float64(logits[0]), float64(logits[1])), nil
}

// softmax2 reproduces pkg/classifier.softmax2 — return the probability
// of class 1 under a binary softmax.
func softmax2(a, b float64) float64 {
	m := math.Max(a, b)
	ea, eb := math.Exp(a-m), math.Exp(b-m)
	return eb / (ea + eb)
}

// BatchScore scores multiple candidates in a single transformer forward
// pass. It tokenizes each triple, pads each example to the batch's max
// encoded length (capped by maxLen), stacks the result into [B, T] int32
// arrays, and runs Model.ForwardBatch once.
//
// Returns probabilities in the same order as the input. An empty input
// yields an empty result. Numerically matches calling Score on each
// triple to within fp32 reassociation drift; see scanner.Scan for the
// fallback path used when a scorer doesn't implement this.
//
// Satisfies scanner.BatchScorer.
func (s *Scorer) BatchScore(triples []scanner.SpanTriple) ([]float64, error) {
	if len(triples) == 0 {
		return nil, nil
	}
	B := len(triples)

	// Encode each triple; remember pre-pad length so we can find the
	// batch's max length without capping below the longest.
	idsList := make([][]int, B)
	maskList := make([][]int, B)
	maxT := 0
	for i, tr := range triples {
		text := tr.Left + "[CAND]" + tr.Span + "[/CAND]" + tr.Right
		enc, err := s.tk.EncodeSingle(text, true)
		if err != nil {
			return nil, fmt.Errorf("encode[%d]: %w", i, err)
		}
		ids := enc.Ids
		mask := enc.AttentionMask
		if len(ids) > s.maxLen {
			ids = ids[:s.maxLen]
			mask = mask[:s.maxLen]
		}
		idsList[i] = ids
		maskList[i] = mask
		if len(ids) > maxT {
			maxT = len(ids)
		}
	}
	if maxT == 0 {
		// Edge case: all-empty tokenization; return zero probabilities.
		out := make([]float64, B)
		return out, nil
	}

	// Pad each to maxT (RoBERTa pad_token_id=1, mask=0).
	idsFlat := make([]int32, B*maxT)
	maskFlat := make([]int32, B*maxT)
	for i := 0; i < B; i++ {
		off := i * maxT
		ids := idsList[i]
		mask := maskList[i]
		for j, v := range ids {
			idsFlat[off+j] = int32(v)
			maskFlat[off+j] = int32(mask[j])
		}
		for j := len(ids); j < maxT; j++ {
			idsFlat[off+j] = 1 // pad token
			// maskFlat already 0
		}
	}

	logits := s.model.ForwardBatch(idsFlat, maskFlat, B)
	if len(logits) != B {
		return nil, fmt.Errorf("ForwardBatch returned %d, want %d", len(logits), B)
	}
	out := make([]float64, B)
	for i, lg := range logits {
		if len(lg) != 2 {
			return nil, fmt.Errorf("expected 2 logits at %d, got %d", i, len(lg))
		}
		out[i] = softmax2(float64(lg[0]), float64(lg[1]))
	}
	return out, nil
}
