package scanner

import (
	"sort"
	"strings"

	"github.com/valllabh/hush/pkg/extractor"
)

// Finding is the output unit: one detected secret.
type Finding struct {
	File       string  `json:"file,omitempty"`
	Line       int     `json:"line"`
	Column     int     `json:"column"`
	Rule       string  `json:"rule"`
	Span       string  `json:"span,omitempty"`
	Redacted   string  `json:"redacted"`
	Start      int     `json:"start"`
	End        int     `json:"end"`
	Entropy    float64 `json:"entropy"`
	Confidence float64 `json:"confidence,omitempty"`
}

// Scorer returns a probability [0,1] that a candidate is a real secret.
// Nil scorer means no ML filtering; all candidates become findings.
type Scorer interface {
	Score(left, span, right string) (float64, error)
}

// Scan finds candidates, optionally filters with the model, returns findings.
func Scan(text string, threshold float64, entropyThreshold float64, ctxChars int, scorer Scorer) ([]Finding, error) {
	cands := extractor.Extract(text, ctxChars, entropyThreshold)
	out := make([]Finding, 0, len(cands))
	for _, c := range cands {
		conf := 1.0
		if scorer != nil {
			p, err := scorer.Score(c.LeftCtx, c.Span, c.RightCtx)
			if err != nil {
				return nil, err
			}
			conf = p
			if p < threshold {
				continue
			}
		}
		out = append(out, Finding{
			Line:       c.Line,
			Column:     c.Column,
			Rule:       c.SourceRule,
			Span:       c.Span,
			Redacted:   extractor.Redact(c.Span),
			Start:      c.Start,
			End:        c.End,
			Entropy:    c.Entropy,
			Confidence: conf,
		})
	}
	return out, nil
}

// MaskText replaces each finding's span with a placeholder. Non-overlapping
// findings expected (extractor dedupes). Returns masked text.
func MaskText(text string, findings []Finding, placeholder string) string {
	if len(findings) == 0 {
		return text
	}
	// Sort by start so we can iterate left-to-right.
	sorted := make([]Finding, len(findings))
	copy(sorted, findings)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i].Start < sorted[j].Start })

	var b strings.Builder
	b.Grow(len(text))
	prev := 0
	for i, f := range sorted {
		if f.Start < prev {
			continue // safety against overlap
		}
		b.WriteString(text[prev:f.Start])
		if placeholder == "" {
			b.WriteString("[REDACTED_")
			b.WriteString(strings.ToUpper(f.Rule))
			b.WriteByte('_')
			b.WriteString(itoa(i + 1))
			b.WriteByte(']')
		} else {
			b.WriteString(placeholder)
		}
		prev = f.End
	}
	b.WriteString(text[prev:])
	return b.String()
}

func itoa(i int) string {
	if i == 0 {
		return "0"
	}
	var buf [20]byte
	n := len(buf)
	for i > 0 {
		n--
		buf[n] = byte('0' + i%10)
		i /= 10
	}
	return string(buf[n:])
}
