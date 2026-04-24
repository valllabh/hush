//go:build native

// Package bundled wires the embedded BitNet classifier into pkg/scanner.
//
// Built with `-tags=native`, this file registers the pure-Go runtime
// from pkg/native instead of the ORT-backed pkg/classifier. No CGO
// and no libonnxruntime dependency in the resulting binary.
package bundled

import (
	"github.com/valllabh/hush/pkg/native"
	"github.com/valllabh/hush/pkg/scanner"
)

func init() {
	scanner.DefaultScorerFactory = func(threads int) (scanner.Scorer, func() error, error) {
		_ = threads // native runtime is single-threaded per Scorer today
		s, err := native.NewBundledScorer()
		if err != nil {
			return nil, nil, err
		}
		return s, s.Close, nil
	}
}
