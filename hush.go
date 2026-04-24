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
// The default build uses the pure Go runtime from pkg/native — no CGO,
// no libonnxruntime, truly static. Advanced users who want to drive the
// extractor or classifier directly can reach pkg/{scanner,native,extractor}.
package hush

import (
	"github.com/valllabh/hush/pkg/native"
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
const ModelVersion = native.ModelVersion

// New returns a Scanner ready to use. By default it loads the embedded
// BitNet classifier via the pure Go runtime. Set Options.ModelOff to
// skip the model and run the extractor only (faster, more false
// positives, zero classifier cost).
func New(opts Options) (*Scanner, error) {
	ensureScorerFactoryRegistered()
	return scanner.New(opts)
}

// Default returns a Scanner with sensible defaults (MinConfidence 0.9,
// embedded model). Useful for one-shot scripts and examples.
func Default() (*Scanner, error) {
	return New(Options{MinConfidence: 0.9})
}

// ensureScorerFactoryRegistered wires the embedded pure-Go classifier
// into pkg/scanner the first time hush.New is called.
func ensureScorerFactoryRegistered() {
	if scanner.DefaultScorerFactory != nil {
		return
	}
	scanner.DefaultScorerFactory = func(threads int) (scanner.Scorer, func() error, error) {
		_ = threads // native runtime is single-threaded per Scorer
		s, err := native.NewBundledScorer()
		if err != nil {
			return nil, nil, err
		}
		return s, s.Close, nil
	}
}
