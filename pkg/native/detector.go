package native

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"sort"
	"strconv"
	"sync"

	"github.com/sugarme/tokenizer"
	"github.com/sugarme/tokenizer/pretrained"
)

// Detector wraps a token-classification (NER) Model and tokenizer and
// produces character-offset spans over arbitrary input text. It owns
// the sliding-window strategy for inputs that exceed the model's
// effective context.
//
// Concurrent use: as of v0.1.11, Detect serializes calls through an
// internal mutex so library callers can wire one Detector into a worker
// pool without crashing on tensor-buffer races. Throughput is therefore
// bounded by one forward pass at a time; callers that need parallel
// throughput should construct N Detectors and round-robin across them.
type Detector struct {
	model  *Model
	tk     *tokenizer.Tokenizer
	maxLen int

	// cached so we don't rebuild per call
	id2label map[int]string
	numLabel int

	// mu serializes Detect calls; the underlying Model mutates tensor
	// buffers during Forward and is not goroutine-safe.
	mu sync.Mutex
}

// Sliding-window character bounds. Tuned so a single window comfortably
// fits inside maxLen=384 RoBERTa tokens; overlap is large enough that a
// real-world secret straddling a boundary still appears intact in one
// window.
const (
	detectorWindowChars = 1500
	detectorWindowStrid = 200
)

// NewDetector returns a Detector using an already-loaded token-classification
// Model and tokenizer. Returns an error if the model isn't a token
// classification model or its label metadata is missing.
func NewDetector(model *Model, tk *tokenizer.Tokenizer) (*Detector, error) {
	if !model.Meta.IsTokenClassification() {
		return nil, fmt.Errorf("native: model is not a token-classification model (task=%q)", model.Meta.Task)
	}
	if model.Meta.Labels == nil {
		return nil, fmt.Errorf("native: token-classification model missing Labels metadata")
	}
	id2 := make(map[int]string, len(model.Meta.Labels.Id2Label))
	for k, v := range model.Meta.Labels.Id2Label {
		id, err := strconv.Atoi(k)
		if err != nil {
			return nil, fmt.Errorf("native: bad id2label key %q: %w", k, err)
		}
		id2[id] = v
	}
	K := model.Meta.Labels.NumLabels
	if K <= 0 {
		return nil, fmt.Errorf("native: NumLabels must be > 0, got %d", K)
	}
	return &Detector{
		model:    model,
		tk:       tk,
		maxLen:   model.Meta.SeqLen,
		id2label: id2,
		numLabel: K,
	}, nil
}

// LoadDetector reads a v2 token-classification Model from modelPath
// (.hbin) and a tokenizer from tokenizerPath (HF tokenizer.json).
func LoadDetector(modelPath, tokenizerPath string) (*Detector, error) {
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
	return NewDetector(m, tk)
}

// LoadDetectorReader is the io.Reader variant for embedded bytes.
func LoadDetectorReader(modelR io.Reader, tokenizerR io.Reader) (*Detector, error) {
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
	return NewDetector(m, tk)
}

// Close is a no-op (no CGO resources). Present for API parity with Scorer.
func (d *Detector) Close() error { return nil }

// ModelVersion returns the embedded model identifier.
func (d *Detector) ModelVersion() string {
	return d.model.Meta.Model
}

// Detect tokenizes text in overlapping char windows, runs Forward on
// each, BIO-decodes the per-token logits, shifts spans back to absolute
// char offsets, and dedupes overlaps across windows. Returns spans
// sorted by Start ascending.
func (d *Detector) Detect(text string) ([]Span, error) {
	if text == "" {
		return nil, nil
	}
	d.mu.Lock()
	defer d.mu.Unlock()

	var all []Span

	step := detectorWindowChars - detectorWindowStrid // 1300
	for start := 0; start < len(text); start += step {
		end := start + detectorWindowChars
		if end > len(text) {
			end = len(text)
		}
		window := text[start:end]
		spans, err := d.detectWindow(window)
		if err != nil {
			return nil, fmt.Errorf("window[%d:%d]: %w", start, end, err)
		}
		for i := range spans {
			spans[i].Start += start
			spans[i].End += start
		}
		all = append(all, spans...)
		if end >= len(text) {
			break
		}
	}

	return dedupSpans(all), nil
}

// detectWindow runs the model on a single window. Returned spans are
// in window-local char offsets.
func (d *Detector) detectWindow(window string) ([]Span, error) {
	enc, err := d.tk.EncodeSingle(window, true)
	if err != nil {
		return nil, fmt.Errorf("encode: %w", err)
	}
	ids := enc.Ids
	mask := enc.AttentionMask
	offs := enc.Offsets

	if len(ids) > d.maxLen {
		ids = ids[:d.maxLen]
		mask = mask[:d.maxLen]
		offs = offs[:d.maxLen]
	}

	// Pad to maxLen with RoBERTa pad_token_id=1, mask=0, (0,0) offsets.
	padOffs := make([][2]int, d.maxLen)
	for i := 0; i < len(offs); i++ {
		if len(offs[i]) >= 2 {
			padOffs[i][0] = offs[i][0]
			padOffs[i][1] = offs[i][1]
		}
	}

	ids32 := make([]int32, d.maxLen)
	mask32 := make([]int32, d.maxLen)
	for i := 0; i < d.maxLen; i++ {
		if i < len(ids) {
			ids32[i] = int32(ids[i])
			mask32[i] = int32(mask[i])
		} else {
			ids32[i] = 1 // pad token
			// mask32[i] already 0
		}
	}

	logits := d.model.Forward(ids32, mask32)
	K := d.numLabel
	if K == 0 || len(logits) == 0 {
		return nil, nil
	}
	effectiveT := len(logits) / K
	if effectiveT == 0 {
		return nil, nil
	}

	// Trim offsets/mask to effective length.
	offTrim := padOffs[:effectiveT]
	maskTrim := mask32[:effectiveT]

	return DecodeBIO(logits, K, d.id2label, offTrim, maskTrim), nil
}

// dedupSpans drops overlapping spans of the same Type (keeping the
// higher-scoring one) and returns the result sorted by Start.
func dedupSpans(spans []Span) []Span {
	if len(spans) <= 1 {
		return spans
	}
	sort.Slice(spans, func(i, j int) bool {
		if spans[i].Start != spans[j].Start {
			return spans[i].Start < spans[j].Start
		}
		return spans[i].End < spans[j].End
	})

	keep := make([]bool, len(spans))
	for i := range keep {
		keep[i] = true
	}
	for i := 0; i < len(spans); i++ {
		if !keep[i] {
			continue
		}
		for j := i + 1; j < len(spans); j++ {
			if !keep[j] {
				continue
			}
			// Sorted by Start, so spans[j].Start >= spans[i].Start. They
			// overlap if spans[j].Start < spans[i].End.
			if spans[j].Start >= spans[i].End {
				break
			}
			if spans[i].Type != spans[j].Type {
				continue
			}
			// Overlap, same type — keep the higher-scoring one.
			if spans[j].Score > spans[i].Score {
				keep[i] = false
				break
			}
			keep[j] = false
		}
	}

	out := spans[:0]
	for i, s := range spans {
		if keep[i] {
			out = append(out, s)
		}
	}
	return out
}
