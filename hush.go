// Package hush is the public, ergonomic entry point for hush.
//
// Most library users should need only this package.
//
// V2 detector (recommended; catches secrets and PII):
//
//	import "github.com/valllabh/hush"
//
//	s, err := hush.New(hush.Options{UseDetector: true, DetectorPrefilter: true})
//	if err != nil { panic(err) }
//	defer s.Close()
//
//	findings, _ := s.ScanReader(reader)
//	masked, _, _ := s.Redact(text, "[REDACTED:%s]")
//
// V1 sequence classifier (legacy, secrets only, faster on huge dirty input):
//
//	s, err := hush.New(hush.Options{MinConfidence: 0.9})
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

// New returns a Scanner ready to use.
//
//   - Options.UseDetector=true:    embedded v2 NER detector (secrets + PII).
//     ModelOff is forced true; v1 classifier is not loaded.
//   - Options.UseDetector=false:   embedded v1 sequence classifier (secrets).
//     This is the legacy path; ModelOff=true skips the model and runs the
//     extractor only.
//
// DetectorPrefilter only affects the v2 path. It enables a regex+entropy
// gate in front of the model so files with no candidates skip the model
// entirely. Strongly recommended (1000x faster on clean files; quality
// is preserved or improved by a hybrid regex+model fusion).
func New(opts Options) (*Scanner, error) {
	if opts.UseDetector {
		opts.ModelOff = true
		s, err := scanner.New(opts)
		if err != nil {
			return nil, err
		}
		det, err := native.NewBundledDetector()
		if err != nil {
			_ = s.Close()
			return nil, err
		}
		s.UseDetector(detectorAdapter{d: det})
		return s, nil
	}

	ensureScorerFactoryRegistered()
	return scanner.New(opts)
}

// Default returns a Scanner with the v2 detector + prefilter enabled.
// Mirrors `hush detect` defaults. Useful for one-shot scripts and
// examples that want sensible behaviour out of the box.
func Default() (*Scanner, error) {
	return New(Options{
		UseDetector:       true,
		DetectorPrefilter: true,
		MinConfidence:     0.5,
		EntropyThreshold:  3.0,
	})
}

// detectorAdapter bridges *native.Detector to scanner.Detector so
// pkg/scanner stays decoupled from pkg/native.
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

// ensureScorerFactoryRegistered wires the embedded pure-Go v1 classifier
// into pkg/scanner the first time hush.New is called without UseDetector.
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
