package scanner_test

import (
	"os"
	"strings"
	"testing"

	"github.com/valllabh/hush/pkg/native"
	"github.com/valllabh/hush/pkg/scanner"
)

const (
	testDetectorModelPath     = "/Users/vajoshi/Work/dump/bitmodel-secrets-detection/models/ner/baseline-v0/onnx/model.int8.hbin"
	testDetectorTokenizerPath = "/Users/vajoshi/Work/dump/bitmodel-secrets-detection/models/ner/baseline-v0/onnx/tokenizer.json"
)

// detectorAdapter bridges native.Detector (which returns []native.Span)
// to the scanner.Detector interface (which expects []scanner.DetectedSpan).
// The scanner package can't import pkg/native directly without a cycle.
type detectorAdapter struct{ d *native.Detector }

func (a detectorAdapter) Detect(text string) ([]scanner.DetectedSpan, error) {
	spans, err := a.d.Detect(text)
	if err != nil {
		return nil, err
	}
	out := make([]scanner.DetectedSpan, len(spans))
	for i, s := range spans {
		out[i] = scanner.DetectedSpan{Start: s.Start, End: s.End, Type: s.Type, Score: s.Score}
	}
	return out, nil
}

func TestScanner_UseDetector_FindsAWSKey(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping detector integration test in -short mode")
	}
	for _, p := range []string{testDetectorModelPath, testDetectorTokenizerPath} {
		if _, err := os.Stat(p); err != nil {
			t.Skipf("detector model asset missing: %s", p)
		}
	}

	d, err := native.LoadDetector(testDetectorModelPath, testDetectorTokenizerPath)
	if err != nil {
		t.Fatalf("LoadDetector: %v", err)
	}
	defer d.Close()

	s, err := scanner.New(scanner.Options{ModelOff: true, MinConfidence: 0.0001})
	if err != nil {
		t.Fatalf("scanner.New: %v", err)
	}
	defer s.Close()
	s.UseDetector(detectorAdapter{d: d})

	text := "AWS_KEY=AKIAIOSFODNN7EXAMPLE in some lorem ipsum text\n"
	findings, err := s.ScanString(text)
	if err != nil {
		t.Fatalf("ScanString: %v", err)
	}
	if len(findings) == 0 {
		t.Fatalf("expected at least one finding, got 0")
	}

	hit := false
	for _, f := range findings {
		if strings.Contains(f.Span, "AKIAIOSFODNN7EXAMPLE") {
			hit = true
		}
		if f.Start < 0 || f.End > len(text) || f.End < f.Start {
			t.Errorf("bad span offsets: %+v", f)
		}
		if f.Rule == "" {
			t.Errorf("finding missing Rule (type): %+v", f)
		}
	}
	if !hit {
		t.Errorf("no finding covered the AKIA token; got %+v", findings)
	}
}
