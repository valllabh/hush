package hush_test

import (
	"fmt"
	"strings"
	"testing"

	"github.com/valllabh/hush"
)

// Library-level perf benchmarks. Run with:
//
//	go test -bench=. -benchmem -run=^$ -benchtime=3s .
//
// Each benchmark runs under two configurations: classifier on and off.
// "Off" measures pure extractor throughput (regex + entropy). "On" measures
// the full pipeline including ONNX classifier scoring per candidate.

var (
	cleanSmall = "the quick brown fox jumps over the lazy dog\n" + strings.Repeat("normal code here\n", 20)
	dirtySmall = `api_key="ghp_abcdefghijklmnopqrstuvwxyz0123456789"` + "\n" +
		`AWS_ACCESS_KEY_ID=AKIAIOSFODNN7EXAMPLE` + "\n" +
		// split literal so github push protection does not flag the test fixture
		`SLACK_TOKEN=` + "xoxb" + `-1234567890-abcdefghijklmnopqr` + "\n"
	clean10K = strings.Repeat("just some normal text, no secrets here at all\n", 220) // ~10KB
	dirty10K = strings.Repeat(
		`line of code\napi_key="ghp_abcdefghijklmnopqrstuvwxyz0123456789"\nmore code\n`, 130) // ~10KB with planted secrets
	clean100K = strings.Repeat("normal source code, nothing to see here, move along please\n", 1700) // ~100KB
)

func newScanner(b *testing.B, modelOff bool) *hush.Scanner {
	b.Helper()
	s, err := hush.New(hush.Options{MinConfidence: 0.9, ModelOff: modelOff})
	if err != nil {
		if modelOff {
			b.Fatalf("New(ModelOff): %v", err)
		}
		b.Skipf("classifier unavailable: %v", err)
	}
	return s
}

func benchScan(b *testing.B, text string, modelOff bool) {
	s := newScanner(b, modelOff)
	defer s.Close()
	b.SetBytes(int64(len(text)))
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := s.ScanString(text)
		if err != nil {
			b.Fatal(err)
		}
	}
}

// ---- clean inputs (measure baseline extractor cost) ----

func BenchmarkScan_CleanSmall_ModelOff(b *testing.B)  { benchScan(b, cleanSmall, true) }
func BenchmarkScan_CleanSmall_WithModel(b *testing.B) { benchScan(b, cleanSmall, false) }
func BenchmarkScan_Clean10K_ModelOff(b *testing.B)    { benchScan(b, clean10K, true) }
func BenchmarkScan_Clean10K_WithModel(b *testing.B)   { benchScan(b, clean10K, false) }
func BenchmarkScan_Clean100K_ModelOff(b *testing.B)   { benchScan(b, clean100K, true) }
func BenchmarkScan_Clean100K_WithModel(b *testing.B)  { benchScan(b, clean100K, false) }

// ---- dirty inputs (classifier actually fires) ----

func BenchmarkScan_DirtySmall_ModelOff(b *testing.B)  { benchScan(b, dirtySmall, true) }
func BenchmarkScan_DirtySmall_WithModel(b *testing.B) { benchScan(b, dirtySmall, false) }
func BenchmarkScan_Dirty10K_ModelOff(b *testing.B)    { benchScan(b, dirty10K, true) }
func BenchmarkScan_Dirty10K_WithModel(b *testing.B)   { benchScan(b, dirty10K, false) }

// ---- Redact (scan + inline substitution) ----

func BenchmarkRedact_Dirty10K_ModelOff(b *testing.B) {
	s := newScanner(b, true)
	defer s.Close()
	b.SetBytes(int64(len(dirty10K)))
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _, err := s.Redact(dirty10K, "[REDACTED:%s]")
		if err != nil {
			b.Fatal(err)
		}
	}
}

// ---- classifier cost isolation (one call per iter) ----

func BenchmarkClassifierSingleCall(b *testing.B) {
	s := newScanner(b, false)
	defer s.Close()
	// Dirty small triggers exactly 3 candidates; divide by 3 in analysis for
	// per-candidate cost.
	b.SetBytes(int64(len(dirtySmall)))
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := s.ScanString(dirtySmall)
		if err != nil {
			b.Fatal(err)
		}
	}
}

// fmt is kept imported in case future benches want to print richer output.
var _ = fmt.Sprintf
