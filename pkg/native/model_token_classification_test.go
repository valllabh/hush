package native

import (
	"os"
	"testing"
)

// Note: the int8.hbin is currently produced with a broken classifier
// transpose for token-classification heads (the exporter assumes Gemm
// layout but the v2 head exports as MatMul). We use the fp32 hbin here
// which has the correct [in, out] layout. Re-enable int8 once the
// exporter is fixed.
const v2HBinPath = "/Users/vajoshi/Work/dump/bitmodel-secrets-detection/models/ner/baseline-v0/onnx/model.hbin"

// TestForwardTokenClassification loads the real v2 NER hbin and verifies
// Forward returns T*num_labels logits (instead of OutputClasses).
func TestForwardTokenClassification(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping under -short")
	}
	f, err := os.Open(v2HBinPath)
	if err != nil {
		t.Skipf("v2 hbin not available at %s: %v", v2HBinPath, err)
	}
	defer f.Close()

	bundle, err := Read(f)
	if err != nil {
		t.Fatalf("Read: %v", err)
	}
	if !bundle.Meta.IsTokenClassification() {
		t.Fatalf("expected token_classification, got Task=%q", bundle.Meta.Task)
	}
	if bundle.Meta.Labels == nil || bundle.Meta.Labels.NumLabels == 0 {
		t.Fatalf("expected non-zero NumLabels, got %+v", bundle.Meta.Labels)
	}
	numLabels := bundle.Meta.Labels.NumLabels

	model, err := LoadModel(bundle)
	if err != nil {
		t.Fatalf("LoadModel: %v", err)
	}
	if model.ClassifierW == nil || len(model.ClassifierB) != numLabels {
		t.Fatalf("token classification head not loaded: W=%v B-len=%d", model.ClassifierW, len(model.ClassifierB))
	}
	// v1 heads must NOT be populated for v2 model.
	if model.ClsDenseW != nil || model.ClsOutW != nil {
		t.Fatalf("v1 classifier heads should be nil for token classification model")
	}

	// Build a short padded input. Effective length T = 4 (mask trims trailing pads).
	seqLen := 16
	inputIDs := make([]int32, seqLen)
	mask := make([]int32, seqLen)
	inputIDs[0] = 0   // <s>
	inputIDs[1] = 100 // arbitrary token
	inputIDs[2] = 200
	inputIDs[3] = 2 // </s>
	for i := 0; i < 4; i++ {
		mask[i] = 1
	}
	// remaining positions are pad (id=1, mask=0)
	for i := 4; i < seqLen; i++ {
		inputIDs[i] = 1
	}

	out := model.Forward(inputIDs, mask)
	wantT := 4
	if got, want := len(out), wantT*numLabels; got != want {
		t.Fatalf("Forward len = %d, want T*num_labels = %d*%d = %d", got, wantT, numLabels, want)
	}

	// ForwardBatch with B=1 should match length.
	batchOut := model.ForwardBatch(inputIDs, mask, 1)
	if len(batchOut) != 1 {
		t.Fatalf("ForwardBatch returned %d slots, want 1", len(batchOut))
	}
	if got, want := len(batchOut[0]), wantT*numLabels; got != want {
		t.Fatalf("ForwardBatch[0] len = %d, want %d", got, want)
	}
}
