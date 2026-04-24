// Package classifier wraps the embedded BitNet ONNX model.
//
// The model and tokenizer are compiled into the binary via //go:embed so the
// hush binary is truly portable (user still needs libonnxruntime at runtime).
package classifier

import (
	"bytes"
	_ "embed"
	"fmt"
	"math"
	"os"
	"path/filepath"

	"github.com/sugarme/tokenizer"
	"github.com/sugarme/tokenizer/pretrained"
	ort "github.com/yalue/onnxruntime_go"
)

// ModelVersion is the version string of the embedded model, exposed so
// callers can log / report which model they are running.
const ModelVersion = "v1"

//go:embed assets/models/hush-model-v1.onnx
var modelBytes []byte

//go:embed assets/models/hush-model-v1.tokenizer.json
var tokenizerJSON []byte

const (
	maxLen = 256
)

// Classifier holds an ONNX session + tokenizer. Safe for concurrent use.
type Classifier struct {
	sess *ort.DynamicAdvancedSession
	tk   *tokenizer.Tokenizer
}

// New loads the embedded model and tokenizer. intraOpThreads caps the ORT
// intra-op thread pool so hush honours worker limits (ORT otherwise ignores
// GOMAXPROCS and spawns NumCPU threads per session). Pass <= 0 for default.
func New(intraOpThreads int) (*Classifier, error) {
	if err := initORT(); err != nil {
		return nil, err
	}

	// Materialise embedded model to a temp file; onnxruntime_go needs a path.
	tmp, err := os.CreateTemp("", "hush-*.onnx")
	if err != nil {
		return nil, fmt.Errorf("temp file: %w", err)
	}
	if _, err := tmp.Write(modelBytes); err != nil {
		return nil, fmt.Errorf("writing model: %w", err)
	}
	tmp.Close()

	sessOpts, err := ort.NewSessionOptions()
	if err != nil {
		return nil, fmt.Errorf("session options: %w", err)
	}
	if intraOpThreads > 0 {
		sessOpts.SetIntraOpNumThreads(intraOpThreads)
		sessOpts.SetInterOpNumThreads(1)
	}
	defer sessOpts.Destroy()

	sess, err := ort.NewDynamicAdvancedSession(
		tmp.Name(),
		[]string{"input_ids", "attention_mask"},
		[]string{"logits"},
		sessOpts,
	)
	if err != nil {
		return nil, fmt.Errorf("ort session: %w", err)
	}

	// Load tokenizer from embedded JSON bytes.
	tk, err := pretrained.FromReader(bytes.NewReader(tokenizerJSON))
	if err != nil {
		return nil, fmt.Errorf("tokenizer: %w", err)
	}

	return &Classifier{sess: sess, tk: tk}, nil
}

// Close releases the ONNX session.
func (c *Classifier) Close() error {
	if c == nil || c.sess == nil {
		return nil
	}
	return c.sess.Destroy()
}

// Score returns the probability [0,1] that the span is a real secret.
func (c *Classifier) Score(left, span, right string) (float64, error) {
	text := left + "[CAND]" + span + "[/CAND]" + right
	enc, err := c.tk.EncodeSingle(text, true)
	if err != nil {
		return 0, fmt.Errorf("encode: %w", err)
	}
	ids := enc.Ids
	mask := enc.AttentionMask

	if len(ids) > maxLen {
		ids = ids[:maxLen]
		mask = mask[:maxLen]
	}
	// Pad to maxLen.
	for len(ids) < maxLen {
		ids = append(ids, 1) // pad_token_id = 1 for RoBERTa
		mask = append(mask, 0)
	}

	idsT := make([]int64, maxLen)
	maskT := make([]int64, maxLen)
	for i := 0; i < maxLen; i++ {
		idsT[i] = int64(ids[i])
		maskT[i] = int64(mask[i])
	}

	inShape := ort.NewShape(1, int64(maxLen))
	idsTensor, err := ort.NewTensor(inShape, idsT)
	if err != nil {
		return 0, err
	}
	defer idsTensor.Destroy()
	maskTensor, err := ort.NewTensor(inShape, maskT)
	if err != nil {
		return 0, err
	}
	defer maskTensor.Destroy()

	outShape := ort.NewShape(1, 2)
	outTensor, err := ort.NewEmptyTensor[float32](outShape)
	if err != nil {
		return 0, err
	}
	defer outTensor.Destroy()

	if err := c.sess.Run(
		[]ort.Value{idsTensor, maskTensor},
		[]ort.Value{outTensor},
	); err != nil {
		return 0, fmt.Errorf("run: %w", err)
	}
	logits := outTensor.GetData()
	p := softmax2(float64(logits[0]), float64(logits[1]))
	return p, nil
}

func softmax2(a, b float64) float64 {
	m := math.Max(a, b)
	ea, eb := math.Exp(a-m), math.Exp(b-m)
	return eb / (ea + eb)
}

// initORT is called once; locates libonnxruntime and starts the runtime.
func initORT() error {
	if ort.IsInitialized() {
		return nil
	}
	// Let user override via env var.
	if p := os.Getenv("ONNXRUNTIME_LIB"); p != "" {
		ort.SetSharedLibraryPath(p)
	} else if p := findLib(); p != "" {
		ort.SetSharedLibraryPath(p)
	}
	return ort.InitializeEnvironment()
}

func findLib() string {
	candidates := []string{
		"/opt/homebrew/lib/libonnxruntime.dylib",
		"/usr/local/lib/libonnxruntime.dylib",
		"/usr/lib/x86_64-linux-gnu/libonnxruntime.so",
		"/usr/lib/libonnxruntime.so",
	}
	for _, c := range candidates {
		if _, err := os.Stat(c); err == nil {
			return c
		}
	}
	// brew path with version prefix
	matches, _ := filepath.Glob("/opt/homebrew/Cellar/onnxruntime/*/lib/libonnxruntime.dylib")
	if len(matches) > 0 {
		return matches[0]
	}
	return ""
}
