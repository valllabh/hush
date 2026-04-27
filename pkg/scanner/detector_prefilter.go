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

	// URL-shaped: the high-entropy fallback in pkg/extractor flags long
	// alphanumeric runs inside URLs (e.g. badge / docs links in
	// README.md and CI yaml) as `secret`. Real secrets in URLs are rare
	// and live in known patterns (slack webhook, etc.) which already
	// have dedicated rules. So drop spans that are URLs or sit inside
	// the URL portion of the line.
	reLikelyURL = regexp.MustCompile(`^https?://`)
	reURLOnLine = regexp.MustCompile(`https?://\S+`)

	// "Looks like a code identifier in prose": short, all-letters, no
	// digits, no @, no separator. The v2 NER head sometimes labels a
	// camelCase fragment like `evelText` (sliced out of `PadLevelText`)
	// as PII. Real PII tokens have an @, a digit, or a separator.
	reCodeIdentifier = regexp.MustCompile(`^[A-Za-z]{3,16}$`)

	// Words that indicate a regex hit is illustrative, not real. Matched
	// case-insensitively against the SAME LINE as the candidate, or on an
	// isolated comment line immediately preceding it (see looksLikeExample).
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

	// Placeholder-shape patterns that indicate an obviously non-real value:
	// <YOUR_TOKEN>, <SET_ME>, <your-...>, __TOKEN__, ${VAR}, xxxx+, ....+
	reExamplePlaceholder = regexp.MustCompile(`(?i)(?:` +
		`<(?:set_me|your[_\-][a-z0-9_\-]+|[a-z0-9_\-]*token|[a-z0-9_\-]*secret|[a-z0-9_\-]*key|[a-z0-9_\-]*password|placeholder)>|` +
		`__[a-z0-9_]+__|` +
		`\$\{[a-z0-9_]+\}|` +
		`x{5,}|` +
		`\.{4,}` +
		`)`)
)

func looksLikeNonPII(s string) bool {
	if reLikelyUUID.MatchString(s) || reLikelyHash.MatchString(s) || reLikelyDate.MatchString(s) {
		return true
	}
	if reLikelyURL.MatchString(s) {
		return true
	}
	if reCodeIdentifier.MatchString(s) {
		return true
	}
	return false
}

// inURLContext returns true if a candidate at [start, end) sits inside
// the URL portion of its line. Used to suppress high-entropy hits on
// URL fragments (badge links, docs links, GitHub Actions refs).
func inURLContext(text string, start, end int) bool {
	ls, le := lineBoundsAt(text, start)
	line := text[ls:le]
	matches := reURLOnLine.FindAllStringIndex(line, -1)
	if len(matches) == 0 {
		return false
	}
	relStart := start - ls
	relEnd := end - ls
	for _, m := range matches {
		// Span fully inside this URL?
		if relStart >= m[0] && relEnd <= m[1] {
			return true
		}
	}
	return false
}

// commentPrefixes are the line prefixes treated as a "pure comment line"
// for the purposes of looksLikeExample's preceding-line rule.
var commentPrefixes = []string{"//", "#", "--", "/*", "*", ";", "<!--"}

// isCommentOnlyLine reports whether trimmed line content begins with a
// comment marker, with NO assignment-looking syntax (=, :=, : value, etc).
// A line like `# api_key = "AKIA..."` is NOT comment-only — it has the
// candidate inside it. A line like `# example key, do not use` IS.
func isCommentOnlyLine(line string) bool {
	t := line
	// trim leading whitespace
	for len(t) > 0 && (t[0] == ' ' || t[0] == '\t') {
		t = t[1:]
	}
	if t == "" {
		return false
	}
	matched := false
	for _, p := range commentPrefixes {
		if len(t) >= len(p) && t[:len(p)] == p {
			matched = true
			t = t[len(p):]
			break
		}
	}
	if !matched {
		return false
	}
	// If the comment body itself contains an assignment-looking token, it's
	// likely commented-out code with a real-looking secret, not a marker.
	// Detect `<word>(\s*[:=]\s*)` shapes anywhere in the comment body.
	if reAssignmentLike.MatchString(t) {
		return false
	}
	return true
}

// reAssignmentLike matches a name followed by = or :=  or :  with a
// non-trivial value. Used to disqualify "comment-only" lines that are
// actually commented-out assignments containing a candidate.
var reAssignmentLike = regexp.MustCompile(`[A-Za-z_][A-Za-z0-9_]*\s*(?::=|=|:)\s*\S`)

// lineBoundsAt returns the [lineStart, lineEnd) for the line containing
// byte offset off. lineEnd is the index of the trailing '\n' (or len(text)
// if the line is the last one and unterminated).
func lineBoundsAt(text string, off int) (int, int) {
	if off < 0 {
		off = 0
	}
	if off > len(text) {
		off = len(text)
	}
	ls := off
	for ls > 0 && text[ls-1] != '\n' {
		ls--
	}
	le := off
	for le < len(text) && text[le] != '\n' {
		le++
	}
	return ls, le
}

// looksLikeExample returns true if a candidate at [start, end) sits in a
// context that explicitly marks it as illustrative.
//
// Tightened scope (v0.1.10):
//   - The candidate's own line carries an example marker, OR
//   - The line immediately above is a pure comment line carrying a marker
//     (no assignment-looking content), OR
//   - The candidate's surrounding context contains an explicit placeholder
//     shape (<SET_ME>, __TOKEN__, ${VAR}, xxxxx, ....).
//
// This avoids the v0.1.9 failure where a misleading comment far above a
// real-looking secret line silently dropped the finding.
func looksLikeExample(text string, start, end int) bool {
	ls, le := lineBoundsAt(text, start)
	line := text[ls:le]
	if reExampleMarker.MatchString(line) {
		return true
	}
	if reExamplePlaceholder.MatchString(line) {
		return true
	}
	// Check the immediately preceding line if it's comment-only.
	if ls > 0 {
		// previous line ends at ls-1 (which is '\n'); find its start.
		prevEnd := ls - 1
		prevStart, _ := lineBoundsAt(text, prevEnd)
		// Avoid treating the same line as previous (defensive).
		if prevEnd > prevStart {
			prev := text[prevStart:prevEnd]
			if isCommentOnlyLine(prev) && reExampleMarker.MatchString(prev) {
				return true
			}
		}
	}
	return false
}

// highTrustRules carries names of rules whose match span is so structurally
// unambiguous (e.g. cryptographic block boundaries) that a model verdict
// of "noise" must NOT override the regex hit. See scanWithDetectorPrefilter.
var highTrustRules = map[string]bool{
	"private_key_pem":   true,
	"x509_certificate":  true,
	"pgp_private_key":   true,
	"putty_private_key": true,
}

func isHighTrustRule(name string) bool { return highTrustRules[name] }

// reSoftPrefilterName matches two consecutive capitalized words (e.g.
// "Vallabh Joshi", "Linus Torvalds"). Cheap heuristic to pull pure-prose
// PII lines into the model's view when the regex extractor finds nothing.
var reSoftPrefilterName = regexp.MustCompile(`\b[A-Z][a-z]{1,}\s+[A-Z][a-z]{1,}\b`)

// reSoftPrefilterAddress matches a US street suffix in proximity to a
// number — e.g. "123 Main St", "742 Evergreen Avenue".
var reSoftPrefilterAddress = regexp.MustCompile(`(?i)\b\d{1,6}\s+[A-Za-z][A-Za-z\.\s]{0,40}\b(?:St|Street|Ave|Avenue|Rd|Road|Blvd|Lane|Ln|Drive|Dr)\b`)

// softPrefilterCandidates synthesizes fake regex candidates at line spans
// that look like they could carry contextual PII (names, addresses) but
// did NOT trigger any rule. Without these, pure-prose lines never reach
// the model. Empty input means no soft hits — caller should still skip
// the model when the real-rule extractor also returned empty.
func softPrefilterCandidates(text string) []extractor.Candidate {
	var out []extractor.Candidate
	for _, m := range reSoftPrefilterName.FindAllStringIndex(text, -1) {
		out = append(out, extractor.Candidate{Start: m[0], End: m[1], SourceRule: "soft_name", RuleType: extractor.RuleTypePII})
	}
	for _, m := range reSoftPrefilterAddress.FindAllStringIndex(text, -1) {
		out = append(out, extractor.Candidate{Start: m[0], End: m[1], SourceRule: "soft_address", RuleType: extractor.RuleTypePII})
	}
	return out
}

// scanWithDetectorPrefilter runs the hybrid v2 pipeline:
//
//  1. Run the regex+entropy extractor. If it finds nothing, skip the model
//     entirely (the speed win on clean files).
//  2. Run the detector on windows around each candidate.
//  3. Fuse model spans with regex candidates:
//     - For each regex candidate that the model OVERLAPS and labels as
//     "noise", DROP the candidate (model says it's a fake/example).
//     - For each remaining regex candidate, EMIT it with the regex's class
//     (secret/pii) — the regex knows the class deterministically. This
//     guarantees v2 catches everything v1's regex catches.
//     - For each model span that does NOT overlap any regex candidate,
//     EMIT it (these are the v2 wins: names, custom tokens, contextual
//     PII v1's regex couldn't see).
//  4. Drop model-only spans whose text is obviously not PII (UUIDs,
//     hex hashes, ISO dates) — model precision backstop.
//
// Class labels in output: "secret", "pii", or "noise".
func scanWithDetectorPrefilter(text string, d Detector, minConfidence, entropyThreshold float64) ([]Finding, error) {
	cands := extractor.Extract(text, 0, entropyThreshold)
	soft := softPrefilterCandidates(text)
	if len(cands) == 0 && len(soft) == 0 {
		return nil, nil
	}

	// Build windows from BOTH real and soft candidates so model sees the
	// soft regions too, but only real candidates participate in fusion.
	allCands := make([]extractor.Candidate, 0, len(cands)+len(soft))
	allCands = append(allCands, cands...)
	allCands = append(allCands, soft...)
	windows := buildWindows(allCands, len(text), detectorWindowChars)
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
		// High-trust deterministic rules (PEM private keys, certificates,
		// PGP blocks) must NOT be dropped by a model "noise" verdict on
		// the body bytes. The model commonly mislabels base64 padding
		// inside a PEM as random noise; the BEGIN/END boundary is the
		// authoritative signal.
		if modelSaysNoise && !isHighTrustRule(c.SourceRule) {
			continue
		}
		if looksLikeExample(text, c.Start, c.End) {
			continue
		}
		// Drop high-entropy hits that fall inside a URL (badge URLs in
		// READMEs, docs URLs in CI yaml, etc.) — those are not secrets,
		// they just happen to contain random-looking alphanumeric runs.
		// Strong-signal rules (PEM blocks, AWS prefixes, etc.) bypass
		// this check via isHighTrustRule.
		if c.SourceRule == "high_entropy" && !isHighTrustRule(c.SourceRule) &&
			inURLContext(text, c.Start, c.End) {
			continue
		}
		// Drop spans that look like a non-secret/non-PII string outright
		// (URL, UUID, hash, ISO date, plain code identifier in prose).
		raw := text[c.Start:c.End]
		if c.SourceRule == "high_entropy" && looksLikeNonPII(raw) {
			continue
		}
		ruleClass := c.RuleType // "secret" or "pii"
		if ruleClass == "" {
			ruleClass = "secret"
		}
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
		if inURLContext(text, start, end) {
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
