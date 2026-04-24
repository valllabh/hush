package extractor

import (
	"math"
	"regexp"
)

var highEntropyCharset = regexp.MustCompile(`[A-Za-z0-9+/_\-]{20,}={0,2}`)

// ShannonEntropy computes base-2 Shannon entropy of a string.
func ShannonEntropy(s string) float64 {
	if len(s) == 0 {
		return 0
	}
	counts := make(map[rune]int, 64)
	for _, r := range s {
		counts[r]++
	}
	n := float64(len(s))
	var h float64
	for _, c := range counts {
		p := float64(c) / n
		h -= p * math.Log2(p)
	}
	return h
}

type EntropyHit struct {
	Start, End int
	Span       string
	Entropy    float64
}

// FindHighEntropySpans returns spans matching the charset with entropy >= threshold.
func FindHighEntropySpans(text string, threshold float64) []EntropyHit {
	var out []EntropyHit
	for _, idx := range highEntropyCharset.FindAllStringIndex(text, -1) {
		s := text[idx[0]:idx[1]]
		h := ShannonEntropy(s)
		if h >= threshold {
			out = append(out, EntropyHit{idx[0], idx[1], s, h})
		}
	}
	return out
}
