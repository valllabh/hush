package scanner

import (
	"regexp"
	"sort"

	"github.com/valllabh/hush/pkg/extractor"
)

// detectorWindowChars must match pkg/native.detectorWindowChars. Hardcoded
// here rather than imported to keep pkg/scanner free of the model runtime
// dependency. If the native window size changes, update both.
const detectorWindowChars = 1500

// Patterns the model commonly mislabels as PII but are obviously not.
// Used to drop model-only spans whose text matches one of these.
var (
	reLikelyUUID = regexp.MustCompile(`^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$`)
	reLikelyHash = regexp.MustCompile(`^[0-9a-fA-F]{32,128}$`) // md5/sha1/sha256/sha512 hex
	reLikelyDate = regexp.MustCompile(`^\d{4}-\d{2}-\d{2}(T\d{2}:\d{2}:\d{2}.*)?$`)

	// Words that indicate a regex hit is illustrative, not real. Matched
	// case-insensitively against ±exampleCtxChars around the candidate.
	reExampleMarker = regexp.MustCompile(`(?i)\b(?:` +
		`example\s+(?:key|secret|token|password|value|only)|` +
		`for\s+example|e\.g\.|i\.e\.|` +
		`fake[_ -](?:key|secret|token|password|value|cred|api)|` +
		`dummy[_ -](?:key|secret|token|password|value|api)|` +
		`placeholder|` +
		`test[_ -](?:fixture|only|stub|mock|data)|` +
		`fixture|` +
		`do\s+not\s+use|` +
		`illustrative|` +
		`not\s+a\s+real(?:\s+\w+)?|` +
		`for\s+(?:demo|illustration|demonstration)|` +
		`todo:?\s*replace|` +
		`your[_ -](?:secret|token|key|password|api[_ -]?key)` +
		`)\b`)
)

const exampleCtxChars = 80

func looksLikeNonPII(s string) bool {
	if reLikelyUUID.MatchString(s) || reLikelyHash.MatchString(s) || reLikelyDate.MatchString(s) {
		return true
	}
	return false
}

// looksLikeExample returns true if a candidate at [start, end) sits in a
// context that explicitly marks it as illustrative (README examples, test
// fixtures, placeholder values). Used to suppress regex false positives
// the model isn't reliably labelling as noise.
//
// Only the surrounding context is checked, not the candidate text itself,
// so legitimate example.com/example.org email addresses (RFC 2606) are
// not suppressed by the word "example" appearing inside the email span.
func looksLikeExample(text string, start, end int) bool {
	ls := start - exampleCtxChars
	if ls < 0 {
		ls = 0
	}
	re := end + exampleCtxChars
	if re > len(text) {
		re = len(text)
	}
	left := text[ls:start]
	right := text[end:re]
	return reExampleMarker.MatchString(left) || reExampleMarker.MatchString(right)
}

// scanWithDetectorPrefilter runs the hybrid v2 pipeline:
//
//  1. Run the regex+entropy extractor. If it finds nothing, skip the model
//     entirely (the speed win on clean files).
//  2. Run the detector on windows around each candidate.
//  3. Fuse model spans with regex candidates:
//     - For each regex candidate that the model OVERLAPS and labels as
//       "noise", DROP the candidate (model says it's a fake/example).
//     - For each remaining regex candidate, EMIT it with the regex's class
//       (secret/pii) — the regex knows the class deterministically. This
//       guarantees v2 catches everything v1's regex catches.
//     - For each model span that does NOT overlap any regex candidate,
//       EMIT it (these are the v2 wins: names, custom tokens, contextual
//       PII v1's regex couldn't see).
//  4. Drop model-only spans whose text is obviously not PII (UUIDs,
//     hex hashes, ISO dates) — model precision backstop.
//
// Class labels in output: "secret", "pii", or "noise".
func scanWithDetectorPrefilter(text string, d Detector, minConfidence, entropyThreshold float64) ([]Finding, error) {
	cands := extractor.Extract(text, 0, entropyThreshold)
	if len(cands) == 0 {
		return nil, nil
	}

	windows := buildWindows(cands, len(text), detectorWindowChars)
	var modelSpans []DetectedSpan
	for _, w := range windows {
		ws, we := w[0], w[1]
		spans, err := d.Detect(text[ws:we])
		if err != nil {
			return nil, err
		}
		for i := range spans {
			spans[i].Start += ws
			spans[i].End += ws
		}
		modelSpans = append(modelSpans, spans...)
	}
	modelSpans = dedupDetectedSpans(modelSpans)

	// Fuse: classify each regex candidate against overlapping model spans.
	out := make([]Finding, 0, len(cands)+len(modelSpans))
	regexCovered := make([]bool, len(modelSpans))

	for _, c := range cands {
		modelSaysNoise := false
		for i, sp := range modelSpans {
			if spansOverlap(sp.Start, sp.End, c.Start, c.End) {
				regexCovered[i] = true
				if sp.Type == "noise" && float64(sp.Score) >= minConfidence {
					modelSaysNoise = true
				}
			}
		}
		if modelSaysNoise {
			continue
		}
		if looksLikeExample(text, c.Start, c.End) {
			continue
		}
		ruleClass := c.RuleType // "secret" or "pii"
		if ruleClass == "" {
			ruleClass = "secret"
		}
		raw := text[c.Start:c.End]
		line, col := lineColumn(text, c.Start)
		out = append(out, Finding{
			Line:       line,
			Column:     col,
			Rule:       ruleClass,
			Span:       raw,
			Redacted:   extractor.Redact(raw),
			Start:      c.Start,
			End:        c.End,
			Confidence: 1.0, // regex match = deterministic
		})
	}

	// Add model-only spans (didn't overlap any regex candidate).
	for i, sp := range modelSpans {
		if regexCovered[i] {
			continue
		}
		if sp.Type == "noise" {
			continue
		}
		conf := float64(sp.Score)
		if conf < minConfidence {
			continue
		}
		start, end := clampSpan(sp.Start, sp.End, len(text))
		raw := text[start:end]
		if looksLikeNonPII(raw) {
			continue
		}
		line, col := lineColumn(text, start)
		out = append(out, Finding{
			Line:       line,
			Column:     col,
			Rule:       sp.Type,
			Span:       raw,
			Redacted:   extractor.Redact(raw),
			Start:      start,
			End:        end,
			Confidence: conf,
		})
	}

	sort.Slice(out, func(i, j int) bool { return out[i].Start < out[j].Start })
	return out, nil
}

func spansOverlap(aStart, aEnd, bStart, bEnd int) bool {
	return aStart < bEnd && bStart < aEnd
}

func clampSpan(start, end, textLen int) (int, int) {
	if start < 0 {
		start = 0
	}
	if end > textLen {
		end = textLen
	}
	if end < start {
		end = start
	}
	return start, end
}

// buildWindows expands each candidate into a window of windowChars centered
// on the candidate, clamps to [0, textLen], then merges overlapping or
// adjacent windows. The result is sorted by start offset and contains no
// overlaps.
func buildWindows(cands []extractor.Candidate, textLen, windowChars int) [][2]int {
	if len(cands) == 0 {
		return nil
	}
	raw := make([][2]int, 0, len(cands))
	for _, c := range cands {
		mid := (c.Start + c.End) / 2
		half := windowChars / 2
		s := mid - half
		e := mid + half
		if s < 0 {
			s = 0
		}
		if e > textLen {
			e = textLen
		}
		if e <= s {
			continue
		}
		raw = append(raw, [2]int{s, e})
	}
	if len(raw) == 0 {
		return nil
	}
	sort.Slice(raw, func(i, j int) bool { return raw[i][0] < raw[j][0] })

	merged := raw[:0]
	cur := raw[0]
	for i := 1; i < len(raw); i++ {
		if raw[i][0] <= cur[1] {
			if raw[i][1] > cur[1] {
				cur[1] = raw[i][1]
			}
			continue
		}
		merged = append(merged, cur)
		cur = raw[i]
	}
	merged = append(merged, cur)
	return merged
}

// dedupDetectedSpans drops overlapping spans of the same Type, keeping the
// higher-scoring one. Mirrors pkg/native.dedupSpans (kept here to avoid
// importing pkg/native).
func dedupDetectedSpans(spans []DetectedSpan) []DetectedSpan {
	if len(spans) <= 1 {
		return spans
	}
	sort.Slice(spans, func(i, j int) bool {
		if spans[i].Start != spans[j].Start {
			return spans[i].Start < spans[j].Start
		}
		return spans[i].End < spans[j].End
	})
	keep := make([]bool, len(spans))
	for i := range keep {
		keep[i] = true
	}
	for i := 0; i < len(spans); i++ {
		if !keep[i] {
			continue
		}
		for j := i + 1; j < len(spans); j++ {
			if !keep[j] {
				continue
			}
			if spans[j].Start >= spans[i].End {
				break
			}
			if spans[i].Type != spans[j].Type {
				continue
			}
			if spans[j].Score > spans[i].Score {
				keep[i] = false
				break
			}
			keep[j] = false
		}
	}
	out := spans[:0]
	for i, s := range spans {
		if keep[i] {
			out = append(out, s)
		}
	}
	return out
}
