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

// SpanTriple is one candidate to batch-score: left context, span, right
// context. Re-exported from native semantics for scanner callers that
// only import pkg/scanner.
type SpanTriple struct {
	Left, Span, Right string
}

// BatchScorer is an optional extension to Scorer. When the concrete
// scorer implements it, scanner.Scan collects all candidates first and
// runs one batched transformer forward pass instead of N sequential
// Score calls. Falls back to per-candidate Score otherwise.
type BatchScorer interface {
	BatchScore(triples []SpanTriple) ([]float64, error)
}

// Scan finds candidates, optionally filters with the model, returns findings.
//
// PII candidates (RuleType == "pii") bypass the model entirely: regex
// precision is enough on those, and the current shipped classifier was
// trained on credentials so it would over-suppress real PII findings
// (emails, SSNs, credit cards). PII findings are reported with
// confidence 1.0 from the regex match.
func Scan(text string, threshold float64, entropyThreshold float64, ctxChars int, scorer Scorer) ([]Finding, error) {
	cands := extractor.Extract(text, ctxChars, entropyThreshold)
	if len(cands) == 0 {
		return nil, nil
	}

	// Partition candidates: PII bypasses the model, secrets go to it.
	piiSet := make(map[int]bool, len(cands))
	scoreCands := make([]extractor.Candidate, 0, len(cands))
	scoreIdx := make([]int, 0, len(cands))
	for i, c := range cands {
		if c.RuleType == extractor.RuleTypePII {
			piiSet[i] = true
			continue
		}
		scoreCands = append(scoreCands, c)
		scoreIdx = append(scoreIdx, i)
	}

	// Batched fast path for the secret subset.
	var probs []float64
	if scorer != nil && len(scoreCands) > 0 {
		if bs, ok := scorer.(BatchScorer); ok {
			triples := make([]SpanTriple, len(scoreCands))
			for i, c := range scoreCands {
				triples[i] = SpanTriple{Left: c.LeftCtx, Span: c.Span, Right: c.RightCtx}
			}
			ps, err := bs.BatchScore(triples)
			if err != nil {
				return nil, err
			}
			probs = ps
		}
	}

	out := make([]Finding, 0, len(cands))
	scoreSeq := 0
	for i, c := range cands {
		conf := 1.0
		if !piiSet[i] && scorer != nil {
			var p float64
			var err error
			if probs != nil {
				p = probs[scoreSeq]
				scoreSeq++
			} else {
				p, err = scorer.Score(c.LeftCtx, c.Span, c.RightCtx)
				if err != nil {
					return nil, err
				}
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
