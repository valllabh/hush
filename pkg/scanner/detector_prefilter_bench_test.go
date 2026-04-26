package scanner_test

import (
	"os"
	"strings"
	"testing"

	"github.com/valllabh/hush/pkg/native"
	"github.com/valllabh/hush/pkg/scanner"
)

// 4 KB code-like blob with a few credentials. Same shape as
// pkg/native.benchDocCodeLike so numbers are comparable across packages.
var benchDocPrefilter = `# config.yaml — service deployment
service:
  name: payment-gateway
  region: us-east-1
  replicas: 4
  image: registry.example.com/payments/api:1.42.0
  env:
    - name: AWS_ACCESS_KEY_ID
      value: AKIAIOSFODNN7EXAMPLE
    - name: AWS_SECRET_ACCESS_KEY
      value: wJalrXUtnFEMI/K7MDENG/bPxRfiCYEXAMPLEKEY
    - name: STRIPE_SECRET
      value: ` + "sk_" + "live_" + "TESTONLYTESTONLYTESTONLYTESTONLY" + `
metadata:
  owner: payments-team@example.com
  pager: +1-415-555-2671
  notes: |
    Lorem ipsum dolor sit amet, consectetur adipiscing elit. Sed do eiusmod
    tempor incididunt ut labore et dolore magna aliqua. Ut enim ad minim
    veniam, quis nostrud exercitation ullamco laboris nisi ut aliquip ex
    ea commodo consequat. Duis aute irure dolor in reprehenderit in voluptate
    velit esse cillum dolore eu fugiat nulla pariatur. Excepteur sint
    occaecat cupidatat non proident, sunt in culpa qui officia deserunt
    mollit anim id est laborum.
secrets:
  github_token: ghp_TESTONLYTESTONLYTESTONLYTESTONLYTEST6
` + strings.Repeat("    # padding line for size                                            \n", 30)

// All-prose 4KB doc with no secrets and no PII patterns. Tests that
// prefilter-on cleanly skips the model entirely.
var benchDocClean = strings.Repeat(
	"Lorem ipsum dolor sit amet consectetur adipiscing elit sed do eiusmod tempor "+
		"incididunt ut labore et dolore magna aliqua. ", 40)

func newBenchScanner(b *testing.B, prefilter bool) *scanner.Scanner {
	b.Helper()
	if testing.Short() {
		b.Skip("skipping detector bench in -short mode")
	}
	if _, err := os.Stat(testDetectorModelPath); err != nil {
		b.Skipf("detector model missing: %s", testDetectorModelPath)
	}
	d, err := native.LoadDetector(testDetectorModelPath, testDetectorTokenizerPath)
	if err != nil {
		b.Fatalf("LoadDetector: %v", err)
	}
	s, err := scanner.New(scanner.Options{
		ModelOff:          true,
		MinConfidence:     0.5,
		EntropyThreshold:  3.0,
		DetectorPrefilter: prefilter,
	})
	if err != nil {
		b.Fatalf("scanner.New: %v", err)
	}
	s.UseDetector(detectorAdapter{d: d})
	return s
}

func BenchmarkDetector_NoPrefilter_DirtyDoc(b *testing.B) {
	s := newBenchScanner(b, false)
	defer s.Close()
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := s.ScanString(benchDocPrefilter)
		if err != nil {
			b.Fatalf("ScanString: %v", err)
		}
	}
}

func BenchmarkDetector_Prefilter_DirtyDoc(b *testing.B) {
	s := newBenchScanner(b, true)
	defer s.Close()
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := s.ScanString(benchDocPrefilter)
		if err != nil {
			b.Fatalf("ScanString: %v", err)
		}
	}
}

func BenchmarkDetector_NoPrefilter_CleanDoc(b *testing.B) {
	s := newBenchScanner(b, false)
	defer s.Close()
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := s.ScanString(benchDocClean)
		if err != nil {
			b.Fatalf("ScanString: %v", err)
		}
	}
}

func BenchmarkDetector_Prefilter_CleanDoc(b *testing.B) {
	s := newBenchScanner(b, true)
	defer s.Close()
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := s.ScanString(benchDocClean)
		if err != nil {
			b.Fatalf("ScanString: %v", err)
		}
	}
}
