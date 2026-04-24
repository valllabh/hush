//go:build ort && slow
// +build ort,slow

package classifier

import (
	"sort"
	"testing"
	"time"
)

// TestLatencyP99 enforces the committed p99 budget of 5 ms for a single
// Score call on a short input. Skips when the ONNX Runtime lib is not
// available (CI installs it explicitly).
//
// Run only on demand:
//
//	go test -run TestLatencyP99 -tags=slow ./pkg/classifier/
//
// Without the build tag the test is compiled out via the build constraint
// below, so it does not slow down the default go test loop.

const p99BudgetMs = 5.0

func TestLatencyP99(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping latency budget under -short")
	}
	c := newOrSkip(t)
	defer c.Close()

	const warmup = 10
	const samples = 200

	// Warmup to amortize first load cost.
	for i := 0; i < warmup; i++ {
		if _, err := c.Score("api_key=\"", "AKIAIOSFODNN7EXAMPLE", "\""); err != nil {
			t.Fatal(err)
		}
	}

	durations := make([]time.Duration, samples)
	for i := 0; i < samples; i++ {
		start := time.Now()
		if _, err := c.Score("api_key=\"", "AKIAIOSFODNN7EXAMPLE", "\""); err != nil {
			t.Fatal(err)
		}
		durations[i] = time.Since(start)
	}
	sort.Slice(durations, func(i, j int) bool { return durations[i] < durations[j] })
	p99 := durations[int(float64(samples)*0.99)]
	p99Ms := float64(p99.Microseconds()) / 1000.0

	t.Logf("classifier latency: p50=%s p99=%s (budget %.1f ms)",
		durations[samples/2], p99, p99BudgetMs)
	if p99Ms > p99BudgetMs {
		t.Errorf("p99 latency %.2f ms exceeds budget %.2f ms", p99Ms, p99BudgetMs)
	}
}
