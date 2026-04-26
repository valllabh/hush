package extractor

import (
	"encoding/base64"
	"regexp"
	"sort"
	"strings"
)

const DefaultCtxChars = 256

// Candidate is a span found in the text with context for downstream scoring.
type Candidate struct {
	Span       string `json:"span"`
	LeftCtx    string `json:"left_ctx"`
	RightCtx   string `json:"right_ctx"`
	SourceRule string `json:"rule"`
	// RuleType is "secret" or "pii"; downstream uses it to decide whether
	// to apply the model. PII rules are precise enough that the secret
	// classifier (trained on credentials, not PII) would over-suppress them.
	RuleType string  `json:"rule_type,omitempty"`
	Start    int     `json:"start"`
	End      int     `json:"end"`
	Line     int     `json:"line"`
	Column   int     `json:"column"`
	Entropy  float64 `json:"entropy"`
}

type rawSpan struct {
	start, end int
	value      string
	rule       string
	ruleType   string
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
			rt := r.Type
			if rt == "" {
				rt = RuleTypeSecret
			}
			out = append(out, rawSpan{s, e, text[s:e], r.Name, rt})
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

// reBase64Token matches a chunk of base64 alphabet (A-Z a-z 0-9 + /)
// with optional trailing padding, length divisible by 4, at least 24
// chars long. Used by the encoded-secret pre-pass (#5).
var reBase64Token = regexp.MustCompile(`[A-Za-z0-9+/]{24,}={0,2}`)

// findEncodedSecrets decodes base64 candidates and re-runs ruleSpans on
// the decoded bytes. When a known rule matches the decoded form, the
// ORIGINAL encoded span is emitted as a candidate carrying that rule's
// metadata. Caps total decode work at 10x the input length so a
// pathological "all-base64" input cannot blow up. Plan item #5.
func findEncodedSecrets(text string) []rawSpan {
	limit := 10 * len(text)
	work := 0
	var out []rawSpan
	for _, m := range reBase64Token.FindAllStringIndex(text, -1) {
		s, e := m[0], m[1]
		token := text[s:e]
		// Length must be divisible by 4 to be valid base64.
		if len(token)%4 != 0 {
			continue
		}
		work += len(token)
		if work > limit {
			break
		}
		// StdEncoding handles `+/` alphabet. Skip on decode error.
		decoded, err := base64.StdEncoding.DecodeString(token)
		if err != nil {
			continue
		}
		// Re-run rules on the decoded text. Skip recursion: only check
		// raw regex rules, not entropy or another base64 pass.
		inner := ruleSpans(string(decoded))
		if len(inner) == 0 {
			continue
		}
		for _, hit := range inner {
			out = append(out, rawSpan{
				start:    s,
				end:      e,
				value:    token,
				rule:     "encoded_" + hit.rule,
				ruleType: hit.ruleType,
			})
		}
	}
	return out
}

// Extract runs the pipeline: regex rules first (they win on overlap), then
// base64-encoded-secret pre-pass, then high-entropy fallback, then dedupe
// + build Candidate records.
func Extract(text string, ctxChars int, entropyThreshold float64) []Candidate {
	if ctxChars <= 0 {
		ctxChars = DefaultCtxChars
	}

	rawRules := ruleSpans(text)
	rawEncoded := findEncodedSecrets(text)
	rawEntropy := FindHighEntropySpans(text, entropyThreshold)
	entropySpans := make([]rawSpan, 0, len(rawEntropy))
	for _, h := range rawEntropy {
		// high-entropy fallback is treated as secret-class — generic
		// random-looking strings are usually credentials, not PII.
		entropySpans = append(entropySpans, rawSpan{h.Start, h.End, h.Span, "high_entropy", RuleTypeSecret})
	}
	combined := dedupe(append(append(rawRules, rawEncoded...), entropySpans...))

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
			RuleType:   s.ruleType,
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
