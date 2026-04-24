package native

import (
	"bytes"
	"encoding/json"
	"fmt"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	_ "embed"

	"github.com/sugarme/tokenizer/pretrained"
)

// findTrainingRoot locates the parent repo that contains
// training/scripts/compare_native.py. Honours HUSH_TRAINING_ROOT override;
// otherwise walks up from the test's working directory.
func findTrainingRoot(t *testing.T) string {
	t.Helper()
	if v := os.Getenv("HUSH_TRAINING_ROOT"); v != "" {
		if _, err := os.Stat(filepath.Join(v, "training", "scripts", "compare_native.py")); err == nil {
			return v
		}
	}
	wd, err := os.Getwd()
	if err != nil {
		t.Skipf("cannot get wd: %v", err)
		return ""
	}
	dir := wd
	for i := 0; i < 10; i++ {
		cand := filepath.Join(dir, "training", "scripts", "compare_native.py")
		if _, err := os.Stat(cand); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}
	return ""
}

// findONNX finds baseline_fp32.onnx next to model.hbin.
func findONNX(hbinPath string) string {
	dir := filepath.Dir(hbinPath)
	cand := filepath.Join(dir, "baseline_fp32.onnx")
	if _, err := os.Stat(cand); err == nil {
		return cand
	}
	return ""
}

// findTokenizer locates the tokenizer.json embedded asset on disk. It lives
// at hush/pkg/classifier/assets/models/hush-model-v1.tokenizer.json relative
// to the test file, i.e. ../classifier/assets/models/....
func findTokenizer(t *testing.T) string {
	t.Helper()
	cand := "../classifier/assets/models/hush-model-v1.tokenizer.json"
	if _, err := os.Stat(cand); err == nil {
		return cand
	}
	t.Skipf("tokenizer.json not found at %s", cand)
	return ""
}

// findPython picks up the parent repo's venv python if present.
func findPython(trainingRoot string) string {
	if p := os.Getenv("HUSH_PYTHON"); p != "" {
		if _, err := os.Stat(p); err == nil {
			return p
		}
	}
	cand := filepath.Join(trainingRoot, ".venv", "bin", "python")
	if _, err := os.Stat(cand); err == nil {
		return cand
	}
	return ""
}

// TestForwardMatchesORTRealistic tokenizes a realistic hush-shaped input
// (left + [CAND] + span + [/CAND] + right), runs native Forward plus ORT
// on the same token IDs, and asserts the logits match within 1e-4.
func TestForwardMatchesORTRealistic(t *testing.T) {
	hbin := modelPath(t) // skips if missing
	onnx := findONNX(hbin)
	if onnx == "" {
		t.Skipf("baseline_fp32.onnx not next to %s", hbin)
	}
	trainingRoot := findTrainingRoot(t)
	if trainingRoot == "" {
		t.Skip("training/scripts/compare_native.py not found (set HUSH_TRAINING_ROOT)")
	}
	py := findPython(trainingRoot)
	if py == "" {
		t.Skip(".venv python not set up; skipping (set HUSH_PYTHON or run make install in parent repo)")
	}
	script := filepath.Join(trainingRoot, "training", "scripts", "compare_native.py")

	// Load tokenizer from the same JSON asset that classifier embeds.
	tkPath := findTokenizer(t)
	tkFile, err := os.Open(tkPath)
	if err != nil {
		t.Fatalf("open tokenizer: %v", err)
	}
	defer tkFile.Close()
	tk, err := pretrained.FromReader(tkFile)
	if err != nil {
		t.Fatalf("tokenizer: %v", err)
	}

	// Realistic input shape: left + [CAND]span[/CAND] + right.
	left := "api_key = \""
	span := "AKIAIOSFODNN7EXAMPLE"
	right := "\""
	text := left + "[CAND]" + span + "[/CAND]" + right

	enc, err := tk.EncodeSingle(text, true)
	if err != nil {
		t.Fatalf("encode: %v", err)
	}
	ids := enc.Ids
	mask := enc.AttentionMask
	tokLen := len(ids)
	t.Logf("tokenized length (pre-pad): %d", tokLen)

	// Load native model to get target SeqLen.
	f, err := os.Open(hbin)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	bundle, err := Read(f)
	if err != nil {
		t.Fatal(err)
	}
	m, err := LoadModel(bundle)
	if err != nil {
		t.Fatal(err)
	}
	T := m.Meta.SeqLen

	if len(ids) > T {
		ids = ids[:T]
		mask = mask[:T]
	}
	for len(ids) < T {
		ids = append(ids, 1) // RoBERTa pad id
		mask = append(mask, 0)
	}

	// Native forward.
	idsI32 := make([]int32, T)
	maskI32 := make([]int32, T)
	for i := 0; i < T; i++ {
		idsI32[i] = int32(ids[i])
		maskI32[i] = int32(mask[i])
	}
	goLogits := m.Forward(idsI32, maskI32)
	t.Logf("Go logits: %v", goLogits)

	// ORT via python helper.
	absOnnx, _ := filepath.Abs(onnx)
	payload := map[string]interface{}{
		"model":          absOnnx,
		"input_ids":      ids,
		"attention_mask": mask,
	}
	buf, err := json.Marshal(payload)
	if err != nil {
		t.Fatal(err)
	}

	cmd := exec.Command(py, script)
	cmd.Stdin = bytes.NewReader(buf)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		t.Skipf("compare_native.py failed: %v\nstderr: %s", err, stderr.String())
	}

	var result struct {
		Logits []float32 `json:"logits"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &result); err != nil {
		t.Fatalf("parse ORT output: %v\nstdout: %s\nstderr: %s", err, stdout.String(), stderr.String())
	}
	t.Logf("ORT logits: %v", result.Logits)

	if len(result.Logits) != len(goLogits) {
		t.Fatalf("length mismatch: go=%d ort=%d", len(goLogits), len(result.Logits))
	}
	maxDiff := 0.0
	for i := range goLogits {
		d := math.Abs(float64(goLogits[i] - result.Logits[i]))
		if d > maxDiff {
			maxDiff = d
		}
	}
	t.Logf("max abs diff: %s", fmt.Sprintf("%.3e", maxDiff))
	if maxDiff > 1e-4 {
		t.Errorf("logits diverge: go=%v ort=%v maxDiff=%v", goLogits, result.Logits, maxDiff)
	}
}
