//go:build !ort

// Package bundled wires the embedded classifier into pkg/scanner.
//
// Default build: pure-Go runtime from pkg/native. No CGO, no
// libonnxruntime, single static binary. The ORT-backed path from
// pkg/classifier is only compiled when `-tags=ort` is set (used for
// numeric equivalence testing, not for shipping).
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
