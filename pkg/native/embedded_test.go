package native

import (
	"strings"
	"testing"
)

// TestBundledScorer verifies the go:embed'd v1 model + tokenizer load and
// score a known AWS-style key at high confidence without any filesystem
// access.
func TestBundledScorer(t *testing.T) {
	s, err := NewBundledScorer()
	if err != nil {
		t.Fatalf("NewBundledScorer: %v", err)
	}
	defer s.Close()

	p, err := s.Score("api_key = \"", "AKIAIOSFODNN7EXAMPLE", "\"")
	if err != nil {
		t.Fatalf("Score: %v", err)
	}
	t.Logf("bundled secret score: %v", p)
	if p < 0.95 {
		t.Errorf("expected > 0.95 for AKIA key, got %v", p)
	}
}

// TestBundledDetector verifies the go:embed'd v2 NER model + tokenizer load
// and produce a high-confidence "secret" span over an AWS key without any
// filesystem access.
func TestBundledDetector(t *testing.T) {
	d, err := NewBundledDetector()
	if err != nil {
		t.Fatalf("NewBundledDetector: %v", err)
	}
	defer d.Close()

	const text = `api_key = "AKIAIOSFODNN7EXAMPLE"`
	akiaStart := strings.Index(text, "AKIA")
	if akiaStart < 0 {
		t.Fatalf("test fixture missing AKIA token")
	}
	akiaEnd := akiaStart + len("AKIAIOSFODNN7EXAMPLE")

	spans, err := d.Detect(text)
	if err != nil {
		t.Fatalf("Detect: %v", err)
	}
	t.Logf("detected %d spans: %+v", len(spans), spans)

	var found *Span
	for i := range spans {
		if spans[i].Type == "secret" {
			// Pick the span whose extent overlaps the AKIA token.
			if spans[i].Start <= akiaStart && spans[i].End >= akiaEnd-1 {
				found = &spans[i]
				break
			}
			if found == nil {
				found = &spans[i]
			}
		}
	}
	if found == nil {
		t.Fatalf("expected at least one secret span, got %+v", spans)
	}
	if found.Score <= 0.9 {
		t.Errorf("expected secret score > 0.9, got %v", found.Score)
	}
	if found.Start > akiaStart || found.End < akiaEnd-1 {
		t.Errorf("secret span [%d,%d) does not cover AKIA token [%d,%d)",
			found.Start, found.End, akiaStart, akiaEnd)
	}
}
