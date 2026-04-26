// Minimal hush library example.
//
//	go run . < sample.txt
//
// Uses the v2 detector (secrets + PII) with the regex prefilter enabled.
// Files with no candidates skip the model entirely.
package main

import (
	"fmt"
	"os"

	"github.com/valllabh/hush"
)

func main() {
	s, err := hush.New(hush.Options{
		UseDetector:       true, // embedded v2 NER detector
		DetectorPrefilter: true, // skip model on clean text
		MinConfidence:     0.5,
	})
	if err != nil {
		fmt.Fprintln(os.Stderr, "init:", err)
		os.Exit(2)
	}
	defer s.Close()

	findings, err := s.ScanReader(os.Stdin)
	if err != nil {
		fmt.Fprintln(os.Stderr, "scan:", err)
		os.Exit(2)
	}
	for _, f := range findings {
		fmt.Printf("line %d col %d  %-7s  %s  confidence=%.2f\n",
			f.Line, f.Column, f.Rule, f.Redacted, f.Confidence)
	}
	if len(findings) > 0 {
		os.Exit(1)
	}
}
