// Minimal hush library example.
//
//	go run . < sample.txt
package main

import (
	"fmt"
	"os"

	"github.com/valllabh/hush"
)

func main() {
	s, err := hush.New(hush.Options{MinConfidence: 0.9})
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
		fmt.Printf("line %d col %d  %s  confidence=%.2f\n", f.Line, f.Column, f.Rule, f.Confidence)
	}
	if len(findings) > 0 {
		os.Exit(1)
	}
}
