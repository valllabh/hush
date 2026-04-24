// Package hush is the public, ergonomic entry point for hush.
//
// Most library users should need only this package:
//
//	import "github.com/valllabh/hush"
//
//	s, err := hush.New(hush.Options{MinConfidence: 0.9})
//	if err != nil { panic(err) }
//	defer s.Close()
//
//	findings, _ := s.ScanReader(reader)
//	masked, _, _ := s.Redact(text, "[REDACTED:%s]")
//	fmt.Println(hush.ModelVersion)
//
// Advanced users who need to swap the classifier, drive extraction directly,
// or embed a different model can reach the underlying packages at
// github.com/valllabh/hush/pkg/{scanner,classifier,extractor}. Those
// subpackages are stable but generic; prefer hush.* for everyday use.
package hush

import (
	"github.com/valllabh/hush/pkg/classifier"
	"github.com/valllabh/hush/pkg/scanner"
)

// Scanner, Options and Finding are aliased from pkg/scanner so the hush
// namespace is complete on its own. Any value produced by hush can be
// passed to pkg/scanner functions and vice versa.
type (
	Scanner = scanner.Scanner
	Options = scanner.Options
	Finding = scanner.Finding
)

// ModelVersion is the version of the classifier model compiled into this
// build. Surface this in --version output, request logs, or telemetry so
// you can correlate findings with a specific model.
const ModelVersion = classifier.ModelVersion

// New returns a Scanner ready to use. By default it loads the embedded
// BitNet classifier; set Options.ModelOff to skip the model and run the
// extractor only (faster, more false positives, no libonnxruntime needed).
//
// Remember to Close() the scanner to release the ONNX session.
func New(opts Options) (*Scanner, error) {
	ensureScorerFactoryRegistered()
	return scanner.New(opts)
}

// Default returns a Scanner with sensible defaults (MinConfidence 0.9,
// embedded model). Useful for one-shot scripts and examples.
func Default() (*Scanner, error) {
	return New(Options{MinConfidence: 0.9})
}

// ensureScorerFactoryRegistered wires the embedded classifier into
// pkg/scanner the first time hush.New is called. Kept lazy so a program
// that only ever uses hush.New(Options{ModelOff: true}) never drags the
// ONNX Runtime into the link graph until it calls a non ModelOff path.
func ensureScorerFactoryRegistered() {
	if scanner.DefaultScorerFactory != nil {
		return
	}
	scanner.DefaultScorerFactory = func(threads int) (scanner.Scorer, func() error, error) {
		c, err := classifier.New(threads)
		if err != nil {
			return nil, nil, err
		}
		return c, c.Close, nil
	}
}
