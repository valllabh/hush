//go:build ort

// Legacy ORT scorer registration, retained only for -tags=ort builds
// used for numeric equivalence testing against the pure-Go runtime.
// Shipping binaries do not include this path.
package bundled

import (
	"github.com/valllabh/hush/pkg/classifier"
	"github.com/valllabh/hush/pkg/scanner"
)

func init() {
	scanner.DefaultScorerFactory = func(threads int) (scanner.Scorer, func() error, error) {
		c, err := classifier.New(threads)
		if err != nil {
			return nil, nil, err
		}
		return c, c.Close, nil
	}
}
