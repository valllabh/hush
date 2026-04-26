package native

import (
	"os"
	"strings"
	"testing"
)

const (
	v2TokenizerPath = "/Users/vajoshi/Work/dump/bitmodel-secrets-detection/models/ner/baseline-v0/onnx/tokenizer.json"
)

// TestDetectorRejectsV1Model: a model with empty Meta.Task is sequence
// classification; NewDetector must reject it.
func TestDetectorRejectsV1Model(t *testing.T) {
	m := &Model{Meta: Meta{Task: ""}}
	if _, err := NewDetector(m, nil); err == nil {
		t.Fatal("NewDetector accepted v1 model; expected error")
	}
}

func TestDetectorRejectsMissingLabels(t *testing.T) {
	m := &Model{Meta: Meta{Task: "token_classification", Labels: nil}}
	if _, err := NewDetector(m, nil); err == nil {
		t.Fatal("NewDetector accepted token_classification with nil Labels; expected error")
	}
}

// loadRealDetector spins up a Detector against the real on-disk v2 model.
// Returns nil if files are missing (caller skips).
func loadRealDetector(t *testing.T) *Detector {
	t.Helper()
	if testing.Short() {
		t.Skip("skipping under -short")
	}
	if _, err := os.Stat(v2HBinPath); err != nil {
		t.Skipf("v2 hbin not available: %v", err)
	}
	if _, err := os.Stat(v2TokenizerPath); err != nil {
		t.Skipf("v2 tokenizer not available: %v", err)
	}
	d, err := LoadDetector(v2HBinPath, v2TokenizerPath)
	if err != nil {
		t.Fatalf("LoadDetector: %v", err)
	}
	return d
}

func TestDetectorDetectShortText(t *testing.T) {
	d := loadRealDetector(t)
	defer d.Close()

	text := "AWS_SECRET_ACCESS_KEY=AKIAIOSFODNN7EXAMPLE"
	spans, err := d.Detect(text)
	if err != nil {
		t.Fatalf("Detect: %v", err)
	}
	if len(spans) == 0 {
		t.Fatalf("expected at least one span, got 0")
	}
	allowed := map[string]bool{"secret": true, "pii": true, "noise": true}
	for i, s := range spans {
		if s.Score == 0 {
			t.Errorf("span[%d] zero score: %+v", i, s)
		}
		if !allowed[s.Type] {
			t.Errorf("span[%d] unexpected type %q: %+v", i, s.Type, s)
		}
		if s.Start < 0 || s.End > len(text) || s.Start >= s.End {
			t.Errorf("span[%d] bad bounds: %+v (text len %d)", i, s, len(text))
		}
	}
	t.Logf("short-text spans: %+v", spans)
}

func TestDetectorSlidingWindow(t *testing.T) {
	d := loadRealDetector(t)
	defer d.Close()

	// 5000-char synthetic doc with one AWS-key-shaped secret near char 3000.
	lorem := "Lorem ipsum dolor sit amet, consectetur adipiscing elit, sed do eiusmod tempor incididunt ut labore et dolore magna aliqua. "
	var sb strings.Builder
	for sb.Len() < 3000 {
		sb.WriteString(lorem)
	}
	prefix := sb.String()[:3000]
	secret := "AWS_SECRET_ACCESS_KEY=AKIAIOSFODNN7EXAMPLE"
	sb.Reset()
	sb.WriteString(prefix)
	sb.WriteString(secret)
	for sb.Len() < 5000 {
		sb.WriteString(lorem)
	}
	text := sb.String()[:5000]

	secretAt := 3000

	spans, err := d.Detect(text)
	if err != nil {
		t.Fatalf("Detect: %v", err)
	}
	if len(spans) == 0 {
		t.Fatalf("no spans detected in 5k-char doc with secret at %d", secretAt)
	}

	// Look for at least one span whose start is within the secret region.
	var hit *Span
	for i := range spans {
		s := &spans[i]
		if s.Start >= secretAt-50 && s.Start <= secretAt+len(secret)+50 {
			hit = s
			break
		}
	}
	if hit == nil {
		t.Errorf("expected a span near char %d; spans=%+v", secretAt, spans)
	} else {
		t.Logf("hit near secret: %+v", *hit)
	}

	// No two same-type spans should overlap (dedup).
	for i := 0; i < len(spans); i++ {
		for j := i + 1; j < len(spans); j++ {
			a, b := spans[i], spans[j]
			if a.Type != b.Type {
				continue
			}
			if a.Start < b.End && b.Start < a.End {
				t.Errorf("overlapping same-type spans: %+v vs %+v", a, b)
			}
		}
	}
}

func TestDetectorEmptyText(t *testing.T) {
	// We don't need to load a real model for this — but NewDetector requires
	// valid labels. Skip cheaply by going through LoadDetector if available;
	// otherwise build a stub Detector.
	if _, err := os.Stat(v2HBinPath); err != nil {
		// Stub path — bypass model entirely.
		d := &Detector{}
		spans, err := d.Detect("")
		if err != nil || spans != nil {
			t.Fatalf("empty text: spans=%v err=%v", spans, err)
		}
		return
	}
	d := loadRealDetector(t)
	defer d.Close()
	spans, err := d.Detect("")
	if err != nil {
		t.Fatalf("Detect(\"\"): %v", err)
	}
	if len(spans) != 0 {
		t.Fatalf("expected 0 spans, got %+v", spans)
	}
}
