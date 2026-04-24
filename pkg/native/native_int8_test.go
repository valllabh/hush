package native

import (
	"math"
	"os"
	"path/filepath"
	"testing"
)

// int8ModelPath locates models/model_int8.hbin. If it is not present,
// tests that need it are skipped rather than failing so that developers
// without the artifact can still run the fp32 suite.
func int8ModelPath(t *testing.T) string {
	t.Helper()
	candidates := []string{
		"../../../models/model_int8.hbin",
		"../../models/model_int8.hbin",
		filepath.Join(os.Getenv("HUSH_HBIN"), "model_int8.hbin"),
	}
	for _, p := range candidates {
		if p == "" {
			continue
		}
		if _, err := os.Stat(p); err == nil {
			return p
		}
	}
	t.Skip("model_int8.hbin not present; run: python training/scripts/export_hbin.py --int8 models/baseline_fp32.onnx models/model_int8.hbin")
	return ""
}

func loadModelFromPath(t testing.TB, p string) *Model {
	t.Helper()
	f, err := os.Open(p)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	bn, err := Read(f)
	if err != nil {
		t.Fatal(err)
	}
	m, err := LoadModel(bn)
	if err != nil {
		t.Fatal(err)
	}
	return m
}

// TestInt8ForwardMatchesFP32 validates that the int8 quantized runtime
// produces logits close to the fp32 reference (both run through the Go
// native runtime). Tolerance is 5e-2: symmetric per-output-channel int8
// quantization typically introduces 1-2% error on logits of this scale.
func TestInt8ForwardMatchesFP32(t *testing.T) {
	// fp32 golden from TestForwardMatchesORT.
	fp32Want := []float32{0.5780989527702332, -0.7365155816078186}

	p := int8ModelPath(t)
	m := loadModelFromPath(t, p)

	// Sanity: at least one layer weight should actually be int8.
	if !m.Layers[0].QueryW.IsInt8() {
		t.Fatalf("expected QueryW to be int8, got fp32 (wrong hbin?)")
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

	got := m.Forward(ids, mask)
	if len(got) != 2 {
		t.Fatalf("logits len=%d want 2", len(got))
	}

	const tol = 5e-2
	var maxDiff float64
	for i := range fp32Want {
		d := math.Abs(float64(got[i] - fp32Want[i]))
		if d > maxDiff {
			maxDiff = d
		}
		if d > tol {
			t.Errorf("logit[%d]: got %v want %v (diff %v > tol %v)", i, got[i], fp32Want[i], d, tol)
		}
	}
	t.Logf("int8 logits=%v fp32=%v maxDiff=%.6f", got, fp32Want, maxDiff)
}

// BenchmarkForwardInt8 measures int8 forward pass latency for comparison
// against BenchmarkForward (fp32).
func BenchmarkForwardInt8(b *testing.B) {
	var path string
	for _, p := range []string{"../../../models/model_int8.hbin", "../../models/model_int8.hbin"} {
		if _, err := os.Stat(p); err == nil {
			path = p
			break
		}
	}
	if path == "" {
		b.Skip("model_int8.hbin not present")
	}
	m := loadModelFromPath(b, path)

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

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = m.Forward(ids, mask)
	}
}
