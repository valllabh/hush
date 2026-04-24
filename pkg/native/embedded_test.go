package native

import "testing"

// TestBundledScorer verifies the go:embed'd model + tokenizer load and
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
