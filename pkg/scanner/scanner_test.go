package scanner

import (
	"encoding/json"
	"strings"
	"testing"
)

// constScorer returns a fixed probability for every candidate (or an error).
type constScorer struct {
	p   float64
	err error
}

func (s constScorer) Score(left, span, right string) (float64, error) {
	return s.p, s.err
}

func TestScan_NoScorerKeepsEveryCandidate(t *testing.T) {
	text := `api_key="ghp_abcdefghijklmnopqrstuvwxyz0123456789"`
	got, err := Scan(text, 0.5, 4.0, 64, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) == 0 {
		t.Fatal("expected at least one finding without scorer")
	}
	if got[0].Confidence != 1.0 {
		t.Errorf("confidence should default to 1.0 without scorer, got %v", got[0].Confidence)
	}
	if got[0].Redacted == got[0].Span {
		t.Error("redacted value shouldn't equal span")
	}
}

func TestScan_ScorerBelowThresholdDrops(t *testing.T) {
	text := `api_key="ghp_abcdefghijklmnopqrstuvwxyz0123456789"`
	got, err := Scan(text, 0.9, 4.0, 64, constScorer{p: 0.1})
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 0 {
		t.Errorf("expected 0 findings below threshold, got %d", len(got))
	}
}

func TestScan_ScorerAboveThresholdKept(t *testing.T) {
	text := `api_key="ghp_abcdefghijklmnopqrstuvwxyz0123456789"`
	got, err := Scan(text, 0.5, 4.0, 64, constScorer{p: 0.95})
	if err != nil {
		t.Fatal(err)
	}
	if len(got) == 0 {
		t.Fatal("expected findings above threshold")
	}
	if got[0].Confidence != 0.95 {
		t.Errorf("expected confidence=0.95, got %v", got[0].Confidence)
	}
}

func TestMaskText_NonOverlapping(t *testing.T) {
	text := "alpha SECRETA beta SECRETB gamma"
	findings := []Finding{
		{Start: 6, End: 13, Rule: "x"},
		{Start: 19, End: 26, Rule: "y"},
	}
	masked := MaskText(text, findings, "")
	if strings.Contains(masked, "SECRETA") || strings.Contains(masked, "SECRETB") {
		t.Errorf("mask didn't hide values: %s", masked)
	}
	if !strings.HasPrefix(masked, "alpha ") || !strings.HasSuffix(masked, " gamma") {
		t.Errorf("mask damaged surrounding text: %s", masked)
	}
}

func TestMaskText_FixedPlaceholder(t *testing.T) {
	text := "before SECRET after"
	findings := []Finding{{Start: 7, End: 13, Rule: "x"}}
	masked := MaskText(text, findings, "***")
	if masked != "before *** after" {
		t.Errorf("fixed placeholder mismatch: %q", masked)
	}
}

func TestMaskText_EmptyFindings(t *testing.T) {
	text := "nothing to hide"
	if got := MaskText(text, nil, ""); got != text {
		t.Errorf("empty findings should pass through: %q", got)
	}
}

// Regression test for plan item #18: when a finding's [Start,End] is
// narrower than the underlying regex match, MaskText must snap outward
// to the regex span so no edge bytes of the secret leak.
func TestMaskText_SnapsToRegexMatch(t *testing.T) {
	const aws = "AKIAIOSFODNN7EXAMPLE"
	text := "api_key=" + aws + "\n"
	awsStart := strings.Index(text, "AKIA")
	// Simulate a model returning a narrower span (drops the first 3 and
	// last 3 chars). Without snapping, those 6 bytes would leak.
	narrow := []Finding{{Start: awsStart + 3, End: awsStart + len(aws) - 3, Rule: "secret"}}
	masked := MaskText(text, narrow, "***")
	if strings.Contains(masked, "AKI") || strings.Contains(masked, "PLE") {
		t.Errorf("MaskText leaked edge bytes: %q", masked)
	}
	if strings.Contains(masked, aws) {
		t.Errorf("full secret still visible: %q", masked)
	}
}

// Regression test for plan item #19: Finding.MarshalJSON must omit Span
// by default so library callers cannot accidentally leak the raw secret
// via json.Marshal on a Finding.
func TestFinding_MarshalJSON_OmitsSpanByDefault(t *testing.T) {
	f := Finding{
		Line: 1, Column: 1, Rule: "secret",
		Span:     "AKIAIOSFODNN7EXAMPLE",
		Redacted: "AKI***PLE",
		Start:    0, End: 20,
	}
	b, err := json.Marshal(f)
	if err != nil {
		t.Fatal(err)
	}
	s := string(b)
	if strings.Contains(s, "AKIAIOSFODNN7EXAMPLE") {
		t.Errorf("default Finding JSON leaked Span: %s", s)
	}
	if !strings.Contains(s, `"redacted"`) {
		t.Errorf("default Finding JSON missing redacted: %s", s)
	}
}

// Regression test for plan item #19: RevealedFinding wrapper opts back
// in to Span emission for callers that genuinely need it (CLI
// --output-reveal-secrets, key rotation pipelines).
func TestRevealedFinding_MarshalJSON_IncludesSpan(t *testing.T) {
	f := Finding{Line: 1, Column: 1, Rule: "secret", Span: "AKIAIOSFODNN7EXAMPLE", Redacted: "AKI***PLE"}
	b, err := json.Marshal(RevealedFinding{F: f})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(b), "AKIAIOSFODNN7EXAMPLE") {
		t.Errorf("RevealedFinding JSON should include Span: %s", string(b))
	}
}
