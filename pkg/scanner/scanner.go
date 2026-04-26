package scanner

import (
	"encoding/json"
	"sort"
	"strings"

	"github.com/valllabh/hush/pkg/extractor"
)

// Finding is the output unit: one detected secret or PII span.
//
// SECURITY NOTE: Span holds the raw secret/PII value. Library callers
// who serialize findings to logs, dashboards, CI artifacts, telemetry,
// or any external sink should ALWAYS pass through SafeForOutput first
// (or set Span = "" manually). Otherwise hush leaks the very thing it
// was supposed to find. The hush CLI emits findings without Span by
// default; --output-reveal-secrets opts back in.
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

// findingJSON is the wire shape used by Finding.MarshalJSON. Span is
// deliberately absent so the raw secret value never leaks via the default
// json.Marshal path. Library callers who explicitly want the raw value
// (key rotation pipelines) must read Finding.Span directly.
type findingJSON struct {
	File       string  `json:"file,omitempty"`
	Line       int     `json:"line"`
	Column     int     `json:"column"`
	Rule       string  `json:"rule"`
	Redacted   string  `json:"redacted"`
	Start      int     `json:"start"`
	End        int     `json:"end"`
	Entropy    float64 `json:"entropy,omitempty"`
	Confidence float64 `json:"confidence,omitempty"`
}

// MarshalJSON serializes a Finding without the raw Span value. This is the
// safe default: any pipeline that pipes findings to logs/dashboards/CI
// artifacts via json.Marshal cannot accidentally leak the secret it just
// detected. Callers that genuinely need the raw value (rotation,
// revocation) should read f.Span directly and emit it through a private
// sink.
func (f Finding) MarshalJSON() ([]byte, error) {
	return json.Marshal(findingJSON{
		File:       f.File,
		Line:       f.Line,
		Column:     f.Column,
		Rule:       f.Rule,
		Redacted:   f.Redacted,
		Start:      f.Start,
		End:        f.End,
		Entropy:    f.Entropy,
		Confidence: f.Confidence,
	})
}

// RevealedFinding wraps a Finding so json.Marshal includes the raw Span
// value. Use this only when the sink is private and the caller has
// explicitly opted in (e.g. CLI --output-reveal-secrets). Default
// Finding.MarshalJSON intentionally omits Span.
type RevealedFinding struct{ F Finding }

// MarshalJSON on RevealedFinding emits the raw Span. Callers who reach
// for this type are explicitly opting in to a leak risk.
func (r RevealedFinding) MarshalJSON() ([]byte, error) {
	type wire struct {
		File       string  `json:"file,omitempty"`
		Line       int     `json:"line"`
		Column     int     `json:"column"`
		Rule       string  `json:"rule"`
		Span       string  `json:"span,omitempty"`
		Redacted   string  `json:"redacted"`
		Start      int     `json:"start"`
		End        int     `json:"end"`
		Entropy    float64 `json:"entropy,omitempty"`
		Confidence float64 `json:"confidence,omitempty"`
	}
	return json.Marshal(wire{
		File:       r.F.File,
		Line:       r.F.Line,
		Column:     r.F.Column,
		Rule:       r.F.Rule,
		Span:       r.F.Span,
		Redacted:   r.F.Redacted,
		Start:      r.F.Start,
		End:        r.F.End,
		Entropy:    r.F.Entropy,
		Confidence: r.F.Confidence,
	})
}

// SafeForOutput returns a copy of the findings with Span cleared, so the
// raw secret/PII value never leaks via JSON serialization. Use this
// before writing findings to any external sink. Pass-through equivalent
// to setting f.Span = "" on each item.
//
// Note: as of v0.1.10, Finding.MarshalJSON also omits Span by default,
// so SafeForOutput is now belt-and-suspenders. It still clears Span on
// the in-memory struct, which matters for callers that build their own
// non-JSON output paths (text templates, struct printers, etc).
func SafeForOutput(findings []Finding) []Finding {
	out := make([]Finding, len(findings))
	for i, f := range findings {
		f.Span = ""
		out[i] = f
	}
	return out
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
	for i, c := range cands {
		if c.RuleType == extractor.RuleTypePII {
			piiSet[i] = true
			continue
		}
		scoreCands = append(scoreCands, c)
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
//
// SAFETY: each finding's [Start,End] is snapped outward to the regex
// extractor's match at that position before masking. This prevents the
// case where the model returns a narrow span (e.g. just the username of
// a connection-string credential) and the placeholder leaves the leading
// or trailing bytes of the actual secret visible in the output.
func MaskText(text string, findings []Finding, placeholder string) string {
	if len(findings) == 0 {
		return text
	}
	// Sort by start so we can iterate left-to-right.
	sorted := make([]Finding, len(findings))
	copy(sorted, findings)
	for i := range sorted {
		s, e := snapToRegexMatch(text, sorted[i].Start, sorted[i].End)
		sorted[i].Start = s
		sorted[i].End = e
	}
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

// snapToRegexMatch expands [start,end) to cover any extractor regex match
// that overlaps it. Used by MaskText so a model-narrow span never leaves
// edge bytes of the underlying secret unmasked. Returns the original
// bounds when no overlapping match is found.
func snapToRegexMatch(text string, start, end int) (int, int) {
	if start < 0 {
		start = 0
	}
	if end > len(text) {
		end = len(text)
	}
	if end <= start {
		return start, end
	}
	// Search a small window around the finding so we don't scan the full
	// document for every finding. 256 bytes either side is enough to catch
	// every default rule (PEM blocks excepted; for those the model rarely
	// emits a narrower-than-regex span anyway).
	const win = 256
	ws := start - win
	if ws < 0 {
		ws = 0
	}
	we := end + win
	if we > len(text) {
		we = len(text)
	}
	sub := text[ws:we]
	bestS, bestE := start, end
	for _, r := range extractor.ActiveRules() {
		idxs := r.Regex.FindAllStringSubmatchIndex(sub, -1)
		for _, m := range idxs {
			s, e := m[0]+ws, m[1]+ws
			if r.ValueGroup > 0 && len(m) > 2*r.ValueGroup+1 {
				s, e = m[2*r.ValueGroup]+ws, m[2*r.ValueGroup+1]+ws
			}
			if s < 0 || e <= s {
				continue
			}
			// overlap?
			if e <= start || s >= end {
				continue
			}
			if s < bestS {
				bestS = s
			}
			if e > bestE {
				bestE = e
			}
		}
	}
	return bestS, bestE
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
