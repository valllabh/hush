package extractor

import (
	"math"
	"regexp"
)

var highEntropyCharset = regexp.MustCompile(`[A-Za-z0-9+/_\-]{20,}={0,2}`)

// reEmbeddedBlob matches large structural blobs whose body bytes are
// known-noise to the secret detector: data:image/* base64 URIs and
// -----BEGIN CERTIFICATE----- ... -----END CERTIFICATE----- blocks.
// FindHighEntropySpans masks these out before entropy scoring so we
// don't spam findings on inline images or embedded certs (#9).
var reEmbeddedBlob = regexp.MustCompile(`(?s)(?:data:image/[a-zA-Z.+\-]+;base64,[A-Za-z0-9+/=]+|-----BEGIN CERTIFICATE-----.*?-----END CERTIFICATE-----)`)

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
//
// As of v0.1.11, embedded data: URIs and CERTIFICATE PEM blocks are
// excluded so their base64 bodies do not flood the result with entropy
// false positives (#9).
func FindHighEntropySpans(text string, threshold float64) []EntropyHit {
	masked := text
	for _, m := range reEmbeddedBlob.FindAllStringIndex(text, -1) {
		s, e := m[0], m[1]
		// Replace with same-length spaces so all subsequent offsets stay
		// aligned to the original text.
		b := []byte(masked)
		for i := s; i < e && i < len(b); i++ {
			b[i] = ' '
		}
		masked = string(b)
	}
	var out []EntropyHit
	for _, idx := range highEntropyCharset.FindAllStringIndex(masked, -1) {
		s := masked[idx[0]:idx[1]]
		h := ShannonEntropy(s)
		if h >= threshold {
			out = append(out, EntropyHit{idx[0], idx[1], text[idx[0]:idx[1]], h})
		}
	}
	return out
}
