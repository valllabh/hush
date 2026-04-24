package classifier

import (
	"strings"
	"testing"
)

// newOrSkip constructs a classifier or skips the test if libonnxruntime is
// not installed on the host. Classifier tests depend on the shared library;
// CI installs it explicitly, local dev may not.
func newOrSkip(t *testing.T) *Classifier {
	t.Helper()
	c, err := New(2)
	if err != nil {
		if strings.Contains(err.Error(), "onnxruntime") || strings.Contains(err.Error(), "dlopen") {
			t.Skipf("onnxruntime unavailable: %v", err)
		}
		t.Fatalf("New: %v", err)
	}
	return c
}

func TestModelVersionConstant(t *testing.T) {
	if ModelVersion == "" {
		t.Fatal("ModelVersion must not be empty")
	}
	if !strings.HasPrefix(ModelVersion, "v") {
		t.Errorf("ModelVersion = %q, want v-prefixed", ModelVersion)
	}
}

func TestEmbeddedAssets(t *testing.T) {
	if len(modelBytes) == 0 {
		t.Fatal("embedded model is empty")
	}
	if len(tokenizerJSON) == 0 {
		t.Fatal("embedded tokenizer is empty")
	}
	if len(modelBytes) < 1024*1024 {
		t.Errorf("embedded model suspiciously small: %d bytes", len(modelBytes))
	}
}

func TestSoftmax2(t *testing.T) {
	cases := []struct {
		a, b float64
		want float64 // approximate
	}{
		{0, 0, 0.5},
		{-100, 100, 1.0},
		{100, -100, 0.0},
	}
	for _, tc := range cases {
		got := softmax2(tc.a, tc.b)
		if got < tc.want-0.01 || got > tc.want+0.01 {
			t.Errorf("softmax2(%v,%v) = %v, want ~%v", tc.a, tc.b, got, tc.want)
		}
	}
}

func TestScoreRange(t *testing.T) {
	c := newOrSkip(t)
	defer c.Close()

	cases := []struct{ left, span, right string }{
		{"api_key = \"", "AKIAIOSFODNN7EXAMPLE", "\""},
		{"hello ", "world", " goodbye"},
		{"", "", ""},
	}
	for _, tc := range cases {
		p, err := c.Score(tc.left, tc.span, tc.right)
		if err != nil {
			t.Fatalf("Score(%q): %v", tc.span, err)
		}
		if p < 0 || p > 1 {
			t.Errorf("Score out of [0,1]: %v for span %q", p, tc.span)
		}
	}
}

func TestSecretScoresHigherThanProse(t *testing.T) {
	c := newOrSkip(t)
	defer c.Close()

	secret, err := c.Score("aws_key = \"", "AKIAIOSFODNN7EXAMPLE", "\"")
	if err != nil {
		t.Fatal(err)
	}
	prose, err := c.Score("The quick brown ", "fox jumps over", " the lazy dog.")
	if err != nil {
		t.Fatal(err)
	}
	if secret <= prose {
		t.Errorf("expected secret score (%v) > prose score (%v)", secret, prose)
	}
}

func TestCloseNil(t *testing.T) {
	var c *Classifier
	if err := c.Close(); err != nil {
		t.Errorf("Close on nil should be no op, got %v", err)
	}
}
