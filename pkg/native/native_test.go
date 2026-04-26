package native

import (
	"encoding/json"
	"math"
	"os"
	"path/filepath"
	"testing"
)

// modelPath finds the model.hbin in the parent training repo so tests
// can run without shipping a 300MB test artifact.
func modelPath(t *testing.T) string {
	t.Helper()
	candidates := []string{
		"../../../models/model.hbin",
		"../../models/model.hbin",
		filepath.Join(os.Getenv("HUSH_HBIN"), "model.hbin"),
	}
	for _, p := range candidates {
		if p == "" {
			continue
		}
		if _, err := os.Stat(p); err == nil {
			return p
		}
	}
	t.Skip("model.hbin not present; run: python training/scripts/export_hbin.py models/baseline_fp32.onnx models/model.hbin")
	return ""
}

func TestHBinLoad(t *testing.T) {
	p := modelPath(t)
	f, err := os.Open(p)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	b, err := Read(f)
	if err != nil {
		t.Fatal(err)
	}
	if b.Meta.Hidden != 768 {
		t.Errorf("hidden=%d, want 768", b.Meta.Hidden)
	}
	if b.Meta.Layers != 6 {
		t.Errorf("layers=%d, want 6", b.Meta.Layers)
	}
	if len(b.Tensors) == 0 {
		t.Fatal("no tensors loaded")
	}
	// check a few key tensors
	for _, name := range []string{
		"m.roberta.embeddings.word_embeddings.weight",
		"m.roberta.encoder.layer.0.attention.self.query.weight",
		"m.roberta.encoder.layer.0.attention.self.query.bias",
		"m.classifier.out_proj.weight",
	} {
		if _, ok := b.Tensors[name]; !ok {
			t.Errorf("missing tensor: %s", name)
		}
	}
	j, _ := json.Marshal(b.Meta)
	t.Logf("meta: %s  tensors=%d", j, len(b.Tensors))
}

func TestModelLoad(t *testing.T) {
	p := modelPath(t)
	f, err := os.Open(p)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	b, err := Read(f)
	if err != nil {
		t.Fatal(err)
	}
	m, err := LoadModel(b)
	if err != nil {
		t.Fatalf("LoadModel: %v", err)
	}
	if len(m.Layers) != m.Meta.Layers {
		t.Errorf("layer count mismatch")
	}
	if m.WordEmb.Shape[0] != m.Meta.Vocab {
		t.Errorf("word_emb vocab=%d want=%d", m.WordEmb.Shape[0], m.Meta.Vocab)
	}
	if m.WordEmb.Shape[1] != m.Meta.Hidden {
		t.Errorf("word_emb hidden=%d want=%d", m.WordEmb.Shape[1], m.Meta.Hidden)
	}
	t.Logf("loaded model: %d layers, vocab=%d, hidden=%d", m.Meta.Layers, m.Meta.Vocab, m.Meta.Hidden)
}

func TestForwardSmoke(t *testing.T) {
	p := modelPath(t)
	f, err := os.Open(p)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	b, err := Read(f)
	if err != nil {
		t.Fatal(err)
	}
	m, err := LoadModel(b)
	if err != nil {
		t.Fatal(err)
	}

	// Use a small seq (pad to something short so the test is fast) —
	// the smoke test just confirms the forward pass completes and produces
	// finite logits. Numeric validation vs ORT happens in a separate test.
	T := m.Meta.SeqLen // must match ORT static seq len for comparison
	ids := make([]int32, T)
	mask := make([]int32, T)
	// a made-up token sequence that will at least exercise embedding lookup
	ids[0] = 0   // <s>
	ids[1] = 100 // some token
	ids[2] = 200
	ids[3] = 2 // </s>
	for i := 0; i < 4; i++ {
		mask[i] = 1
	}
	for i := 4; i < T; i++ {
		ids[i] = 1 // pad
	}

	logits := m.Forward(ids, mask)
	if len(logits) != m.Meta.OutputClasses {
		t.Fatalf("logits len=%d, want %d", len(logits), m.Meta.OutputClasses)
	}
	for i, v := range logits {
		if math.IsNaN(float64(v)) || math.IsInf(float64(v), 0) {
			t.Errorf("logit[%d] is %v", i, v)
		}
	}
	t.Logf("logits = %v", logits)
}

// TestForwardMatchesORT validates numeric equivalence with ONNX Runtime on
// a known input. Golden logits were captured by running
// training/scripts/validate_native.py against models/baseline_fp32.onnx
// on 2026-04-24. If training retrains and this breaks, recapture via:
//
//	python training/scripts/validate_native.py models/baseline_fp32.onnx
//
// Tolerance is 1e-4 absolute; small numeric drift from op ordering is OK,
// anything larger indicates a real divergence.
func TestForwardMatchesORT(t *testing.T) {
	p := modelPath(t)
	f, err := os.Open(p)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	b, err := Read(f)
	if err != nil {
		t.Fatal(err)
	}
	m, err := LoadModel(b)
	if err != nil {
		t.Fatal(err)
	}

	T := m.Meta.SeqLen
	ids := make([]int32, T)
	mask := make([]int32, T)
	ids[0], ids[1], ids[2], ids[3] = 0, 100, 200, 2
	for i := 0; i < 4; i++ {
		mask[i] = 1
	}
	for i := 4; i < T; i++ {
		ids[i] = 1
	}

	want := []float32{0.5780989527702332, -0.7365155816078186}
	got := m.Forward(ids, mask)

	for i := range want {
		diff := math.Abs(float64(got[i] - want[i]))
		if diff > 1e-4 {
			t.Errorf("logit[%d]: got %v want %v (diff %v)", i, got[i], want[i], diff)
		}
	}
	t.Logf("Go matches ORT: got %v, want %v", got, want)
}
