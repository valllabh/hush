package scanner_test

import (
	"testing"

	_ "github.com/valllabh/hush/pkg/bundled"
	"github.com/valllabh/hush/pkg/scanner"
)

// fixture is representative of a small config file with a couple of
// candidate tokens so the scorer runs at least once per iteration.
const fixture = `# sample config
api_key = "AKIAIOSFODNN7EXAMPLE"
password = "hunter2hunter2hunter2"
note = "nothing interesting here, just prose about pipelines"
secret = "sk_" + "live_abcdefghijklmnopqrstuvwx"
`

// BenchmarkScan_WithBundled boots a Scanner via the registered default
// scorer factory (ORT by default, pure-Go with `-tags=native`) and scans
// the fixture per iteration. Compare ns/op between the two builds:
//
//	go test -bench=BenchmarkScan_WithBundled -benchmem ./pkg/scanner/...
//	go test -tags=native -bench=BenchmarkScan_WithBundled -benchmem ./pkg/scanner/...
func BenchmarkScan_WithBundled(b *testing.B) {
	s, err := scanner.New(scanner.Options{MinConfidence: 0.5})
	if err != nil {
		b.Fatalf("scanner.New: %v", err)
	}
	defer s.Close()

	// warm-up scan so lazy initialisations don't dominate the first iter
	if _, err := s.ScanString(fixture); err != nil {
		b.Fatalf("warm-up scan: %v", err)
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := s.ScanString(fixture); err != nil {
			b.Fatalf("scan: %v", err)
		}
	}
}
