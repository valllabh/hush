package extractor

import (
	"sort"
	"strings"
)

const DefaultCtxChars = 256

// Candidate is a span found in the text with context for downstream scoring.
type Candidate struct {
	Span       string  `json:"span"`
	LeftCtx    string  `json:"left_ctx"`
	RightCtx   string  `json:"right_ctx"`
	SourceRule string  `json:"rule"`
	Start      int     `json:"start"`
	End        int     `json:"end"`
	Line       int     `json:"line"`
	Column     int     `json:"column"`
	Entropy    float64 `json:"entropy"`
}

type rawSpan struct {
	start, end int
	value      string
	rule       string
}

func ruleSpans(text string) []rawSpan {
	var out []rawSpan
	for _, r := range ActiveRules() {
		idxs := r.Regex.FindAllStringSubmatchIndex(text, -1)
		for _, m := range idxs {
			s, e := m[0], m[1]
			if r.ValueGroup > 0 && len(m) > 2*r.ValueGroup+1 {
				s, e = m[2*r.ValueGroup], m[2*r.ValueGroup+1]
			}
			if s < 0 || e < 0 || e > len(text) {
				continue
			}
			out = append(out, rawSpan{s, e, text[s:e], r.Name})
		}
	}
	return out
}

func dedupe(spans []rawSpan) []rawSpan {
	var kept []rawSpan
	for _, s := range spans {
		overlap := false
		for _, k := range kept {
			if !(s.end <= k.start || s.start >= k.end) {
				overlap = true
				break
			}
		}
		if !overlap {
			kept = append(kept, s)
		}
	}
	sort.Slice(kept, func(i, j int) bool { return kept[i].start < kept[j].start })
	return kept
}

func lineCol(text string, pos int) (int, int) {
	line := 1
	lastNL := -1
	for i := 0; i < pos && i < len(text); i++ {
		if text[i] == '\n' {
			line++
			lastNL = i
		}
	}
	return line, pos - lastNL
}

// Extract runs the pipeline: regex rules first (they win on overlap), then
// high-entropy fallback, then dedupe + build Candidate records.
func Extract(text string, ctxChars int, entropyThreshold float64) []Candidate {
	if ctxChars <= 0 {
		ctxChars = DefaultCtxChars
	}

	rawRules := ruleSpans(text)
	rawEntropy := FindHighEntropySpans(text, entropyThreshold)
	entropySpans := make([]rawSpan, 0, len(rawEntropy))
	for _, h := range rawEntropy {
		entropySpans = append(entropySpans, rawSpan{h.Start, h.End, h.Span, "high_entropy"})
	}
	combined := dedupe(append(rawRules, entropySpans...))

	out := make([]Candidate, 0, len(combined))
	for _, s := range combined {
		leftStart := s.start - ctxChars
		if leftStart < 0 {
			leftStart = 0
		}
		rightEnd := s.end + ctxChars
		if rightEnd > len(text) {
			rightEnd = len(text)
		}
		left := text[leftStart:s.start]
		right := text[s.end:rightEnd]

		line, col := lineCol(text, s.start)
		out = append(out, Candidate{
			Span:       s.value,
			LeftCtx:    left,
			RightCtx:   right,
			SourceRule: s.rule,
			Start:      s.start,
			End:        s.end,
			Line:       line,
			Column:     col,
			Entropy:    ShannonEntropy(s.value),
		})
	}
	return out
}

// Redact produces the standard first3+stars+last3 preview of a secret.
func Redact(s string) string {
	if len(s) <= 6 {
		return strings.Repeat("*", len(s))
	}
	return s[:3] + strings.Repeat("*", len(s)-6) + s[len(s)-3:]
}
