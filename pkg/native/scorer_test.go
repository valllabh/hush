package native

import (
	"math"
	"os"
	"path/filepath"
	"testing"
)

// tokenizerPath finds the tokenizer.json shipped alongside pkg/classifier.
// Kept separate from modelPath so both can skip independently.
func tokenizerPath(t *testing.T) string {
	t.Helper()
	candidates := []string{
		"../classifier/assets/models/hush-model-v1.tokenizer.json",
		"../../pkg/classifier/assets/models/hush-model-v1.tokenizer.json",
		filepath.Join(os.Getenv("HUSH_TOKENIZER"), "tokenizer.json"),
	}
	for _, p := range candidates {
		if p == "" {
			continue
		}
		if _, err := os.Stat(p); err == nil {
			return p
		}
	}
	t.Skip("tokenizer.json not found")
	return ""
}

func TestScorer_LoadAndScore(t *testing.T) {
	mp := modelPath(t)
	tp := tokenizerPath(t)
	s, err := LoadScorer(mp, tp)
	if err != nil {
		t.Fatalf("LoadScorer: %v", err)
	}
	defer s.Close()

	// real AWS-style key should score high
	p, err := s.Score("api_key = \"", "AKIAIOSFODNN7EXAMPLE", "\"")
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("secret score: %v", p)
	if p < 0.95 {
		t.Errorf("expected high score for AKIA key, got %v", p)
	}

	// random prose should score low
	p2, err := s.Score("The quick brown ", "fox jumps over", " the lazy dog.")
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("prose score: %v", p2)
	if p2 > 0.5 {
		t.Errorf("expected low score for prose, got %v", p2)
	}

	if math.Abs(p-p2) < 0.3 {
		t.Errorf("secret and prose scores too close: %v vs %v", p, p2)
	}
}

func TestScorer_ModelVersion(t *testing.T) {
	mp := modelPath(t)
	tp := tokenizerPath(t)
	s, err := LoadScorer(mp, tp)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	if v := s.ModelVersion(); v == "" {
		t.Error("ModelVersion is empty")
	}
}
