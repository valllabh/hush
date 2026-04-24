// Package bundled wires the embedded BitNet classifier into pkg/scanner.
//
// Blank-import this package from any binary or test that wants
// scanner.New to load the embedded model by default:
//
//	import (
//	    "github.com/valllabh/hush/pkg/scanner"
//	    _ "github.com/valllabh/hush/pkg/bundled"
//	)
//
//	s, _ := scanner.New(scanner.Options{MinConfidence: 0.9})
//
// Library users who want to stay lightweight (no ORT dependency) should
// set scanner.Options.ModelOff = true and skip this package.
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
