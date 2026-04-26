package scanner

import (
	"errors"
	"reflect"
	"strings"
	"testing"

	"github.com/valllabh/hush/pkg/extractor"
)

func TestBuildWindows(t *testing.T) {
	cases := []struct {
		name     string
		cands    []extractor.Candidate
		textLen  int
		windowSz int
		want     [][2]int
	}{
		{
			name:     "empty input",
			cands:    nil,
			textLen:  1000,
			windowSz: 100,
			want:     nil,
		},
		{
			name:     "single small candidate clamped to text bounds",
			cands:    []extractor.Candidate{{Start: 5, End: 15}},
			textLen:  50,
			windowSz: 100,
			want:     [][2]int{{0, 50}},
		},
		{
			name:     "single candidate centered in larger text",
			cands:    []extractor.Candidate{{Start: 1000, End: 1020}},
			textLen:  10000,
			windowSz: 200,
			want:     [][2]int{{910, 1110}},
		},
		{
			name: "two distant candidates produce two windows",
			cands: []extractor.Candidate{
				{Start: 100, End: 110},
				{Start: 5000, End: 5010},
			},
			textLen:  10000,
			windowSz: 200,
			want:     [][2]int{{5, 205}, {4905, 5105}},
		},
		{
			name: "two close candidates merge into one window",
			cands: []extractor.Candidate{
				{Start: 1000, End: 1010},
				{Start: 1050, End: 1060},
			},
			textLen:  10000,
			windowSz: 200,
			want:     [][2]int{{905, 1155}},
		},
		{
			name: "windows touching at boundary merge",
			cands: []extractor.Candidate{
				{Start: 100, End: 110},
				{Start: 300, End: 310},
			},
			textLen:  10000,
			windowSz: 200,
			want:     [][2]int{{5, 405}},
		},
		{
			name:     "candidate at text start clamps to 0",
			cands:    []extractor.Candidate{{Start: 0, End: 10}},
			textLen:  10000,
			windowSz: 200,
			want:     [][2]int{{0, 105}},
		},
		{
			name:     "candidate at text end clamps to textLen",
			cands:    []extractor.Candidate{{Start: 9990, End: 10000}},
			textLen:  10000,
			windowSz: 200,
			want:     [][2]int{{9895, 10000}},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := buildWindows(tc.cands, tc.textLen, tc.windowSz)
			if !reflect.DeepEqual(got, tc.want) {
				t.Errorf("buildWindows = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestDedupDetectedSpans(t *testing.T) {
	cases := []struct {
		name string
		in   []DetectedSpan
		want []DetectedSpan
	}{
		{
			name: "empty",
			in:   nil,
			want: nil,
		},
		{
			name: "no overlap kept as-is",
			in: []DetectedSpan{
				{Start: 0, End: 10, Type: "secret", Score: 0.9},
				{Start: 20, End: 30, Type: "pii", Score: 0.8},
			},
			want: []DetectedSpan{
				{Start: 0, End: 10, Type: "secret", Score: 0.9},
				{Start: 20, End: 30, Type: "pii", Score: 0.8},
			},
		},
		{
			name: "overlap same type keeps higher score",
			in: []DetectedSpan{
				{Start: 0, End: 10, Type: "secret", Score: 0.9},
				{Start: 5, End: 15, Type: "secret", Score: 0.95},
			},
			want: []DetectedSpan{
				{Start: 5, End: 15, Type: "secret", Score: 0.95},
			},
		},
		{
			name: "overlap different type keeps both",
			in: []DetectedSpan{
				{Start: 0, End: 10, Type: "secret", Score: 0.9},
				{Start: 5, End: 15, Type: "pii", Score: 0.8},
			},
			want: []DetectedSpan{
				{Start: 0, End: 10, Type: "secret", Score: 0.9},
				{Start: 5, End: 15, Type: "pii", Score: 0.8},
			},
		},
		{
			name: "result sorted by start",
			in: []DetectedSpan{
				{Start: 50, End: 60, Type: "pii", Score: 0.9},
				{Start: 10, End: 20, Type: "secret", Score: 0.9},
				{Start: 30, End: 40, Type: "pii", Score: 0.9},
			},
			want: []DetectedSpan{
				{Start: 10, End: 20, Type: "secret", Score: 0.9},
				{Start: 30, End: 40, Type: "pii", Score: 0.9},
				{Start: 50, End: 60, Type: "pii", Score: 0.9},
			},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := dedupDetectedSpans(tc.in)
			if !reflect.DeepEqual(got, tc.want) {
				t.Errorf("dedupDetectedSpans = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestLooksLikeNonPII(t *testing.T) {
	cases := []struct {
		in   string
		want bool
	}{
		{"550e8400-e29b-41d4-a716-446655440000", true},                             // UUID
		{"e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855", true}, // sha256
		{"2026-04-25", true},
		{"2026-04-25T13:45:00Z", true},
		{"AKIAIOSFODNN7EXAMPLE", false},
		{"vallabh@example.com", false},
		{"123-45-6789", false},
	}
	for _, tc := range cases {
		t.Run(tc.in, func(t *testing.T) {
			if got := looksLikeNonPII(tc.in); got != tc.want {
				t.Errorf("looksLikeNonPII(%q) = %v, want %v", tc.in, got, tc.want)
			}
		})
	}
}

func TestLooksLikeExample(t *testing.T) {
	cases := []struct {
		name string
		text string
		want bool
	}{
		{
			name: "example key marker triggers",
			text: "Example key: AKIAIOSFODNN7EXAMPLE — do not use",
			want: true,
		},
		{
			name: "test fixture marker triggers",
			text: "# test fixture\nfake_key = \"AKIAIOSFODNN7EXAMPLE\"",
			want: true,
		},
		{
			name: "real config does not trigger",
			text: "aws:\n  access_key: AKIAIOSFODNN7EXAMPLE\n  region: us-east-1",
			want: false,
		},
		{
			name: "example.com in unrelated email does not trigger",
			text: "access_key: AKIAIOSFODNN7EXAMPLE\nemail: ops@example.com",
			want: false,
		},
		{
			name: "do not use phrase triggers",
			text: "AKIAIOSFODNN7EXAMPLE -- do not use this in production",
			want: true,
		},
	}
	// Pick a candidate position that points at AKIA in each text.
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			start := 0
			for i := 0; i+4 <= len(tc.text); i++ {
				if tc.text[i:i+4] == "AKIA" {
					start = i
					break
				}
			}
			end := start + len("AKIAIOSFODNN7EXAMPLE")
			if got := looksLikeExample(tc.text, start, end); got != tc.want {
				t.Errorf("looksLikeExample(%q at %d) = %v, want %v", tc.text, start, got, tc.want)
			}
		})
	}
}

// Regression test for plan item #2: a misleading example marker on a
// far-above line (not the same line, not the immediately preceding
// comment-only line) must NOT suppress a real-looking secret finding.
func TestLooksLikeExample_MisleadingCommentAboveDoesNotSuppress(t *testing.T) {
	text := `// example key in the docs below
//
// here are the production credentials we actually use:
api_key = "AKIAIOSFODNN7EXAMPLE"`
	start := strings.Index(text, "AKIA")
	end := start + len("AKIAIOSFODNN7EXAMPLE")
	if looksLikeExample(text, start, end) {
		t.Errorf("misleading comment far above must not suppress; want false")
	}
}

// Regression test for plan item #2: an example marker on the SAME line
// as the candidate must still suppress.
func TestLooksLikeExample_SameLineTriggers(t *testing.T) {
	text := `api_key = "AKIAIOSFODNN7EXAMPLE" // example key, do not use`
	start := strings.Index(text, "AKIA")
	end := start + len("AKIAIOSFODNN7EXAMPLE")
	if !looksLikeExample(text, start, end) {
		t.Errorf("same-line marker must trigger; want true")
	}
}

// Regression test for plan item #2: a comment-only line immediately
// preceding the candidate carrying the marker must trigger.
func TestLooksLikeExample_PrecedingCommentLineTriggers(t *testing.T) {
	text := "// example key for the docs\napi_key = \"AKIAIOSFODNN7EXAMPLE\""
	start := strings.Index(text, "AKIA")
	end := start + len("AKIAIOSFODNN7EXAMPLE")
	if !looksLikeExample(text, start, end) {
		t.Errorf("preceding comment-only marker must trigger; want true")
	}
}

// Regression test for plan item #2: a "comment" line that is actually a
// commented-out assignment with a real-looking secret must NOT be treated
// as a marker for the next line.
func TestLooksLikeExample_PrecedingCommentedAssignmentDoesNotTrigger(t *testing.T) {
	text := "# example_secret = \"oldvalue\"\napi_key = \"AKIAIOSFODNN7EXAMPLE\""
	start := strings.Index(text, "AKIA")
	end := start + len("AKIAIOSFODNN7EXAMPLE")
	if looksLikeExample(text, start, end) {
		t.Errorf("preceding commented-out assignment must not suppress; want false")
	}
}

// Regression test for plan item #11: placeholder shapes on the same line
// suppress. <SET_ME>, <your-token>, __TOKEN__, ${VAR}, xxxxx, ....
func TestLooksLikeExample_PlaceholderShapes(t *testing.T) {
	cases := []string{
		`api_key = "AKIAIOSFODNN7EXAMPLE" # <SET_ME>`,
		`api_key = "<your-token>" AKIAIOSFODNN7EXAMPLE`,
		`api_key = "AKIAIOSFODNN7EXAMPLE" __TOKEN__`,
		`api_key = "AKIAIOSFODNN7EXAMPLE" ${SOME_VAR}`,
		`api_key = "AKIAIOSFODNN7EXAMPLE" xxxxxxxxxxx`,
		`api_key = "AKIAIOSFODNN7EXAMPLE" .........`,
	}
	for _, text := range cases {
		t.Run(text, func(t *testing.T) {
			start := strings.Index(text, "AKIA")
			end := start + len("AKIAIOSFODNN7EXAMPLE")
			if !looksLikeExample(text, start, end) {
				t.Errorf("placeholder shape must suppress; got false for %q", text)
			}
		})
	}
}

// Regression test for plan item #3: soft prefilter synthesizes candidates
// for two-cap-word names and US-street-suffix addresses so the model sees
// pure-prose lines.
func TestSoftPrefilterCandidates(t *testing.T) {
	cases := []struct {
		name    string
		text    string
		wantMin int
	}{
		{"name", "Hello, my name is Vallabh Joshi.", 1},
		{"public_figure", "code by Linus Torvalds in 1991", 1},
		{"address_st", "send to 123 Main St in Austin", 1},
		{"address_avenue", "742 Evergreen Avenue, Springfield", 1},
		{"address_blvd_no_match_without_number", "Sunset Blvd is famous", 0},
		{"plain_prose", "the quick brown fox jumps over", 0},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := softPrefilterCandidates(tc.text)
			if len(got) < tc.wantMin {
				t.Errorf("softPrefilterCandidates(%q) = %d cands, want >= %d", tc.text, len(got), tc.wantMin)
			}
		})
	}
}

// stubDetector tracks how many times Detect was called and what spans
// to return. Used to verify the prefilter skips the model entirely on
// clean text.
type stubDetector struct {
	calls int
	spans []DetectedSpan
	err   error
}

func (s *stubDetector) Detect(text string) ([]DetectedSpan, error) {
	s.calls++
	if s.err != nil {
		return nil, s.err
	}
	return s.spans, nil
}

func TestPrefilter_SkipsModelOnCleanText(t *testing.T) {
	stub := &stubDetector{}
	findings, err := scanWithDetectorPrefilter(
		"This is a paragraph of plain prose with no secrets and no PII patterns. "+
			"Just lorem ipsum dolor sit amet consectetur adipiscing elit.",
		stub, 0.5, 4.0,
	)
	if err != nil {
		t.Fatalf("scanWithDetectorPrefilter: %v", err)
	}
	if len(findings) != 0 {
		t.Errorf("expected no findings, got %v", findings)
	}
	if stub.calls != 0 {
		t.Errorf("expected detector.Detect to be called 0 times on clean text, got %d", stub.calls)
	}
}

func TestPrefilter_PropagatesDetectorError(t *testing.T) {
	wantErr := errors.New("detector failed")
	stub := &stubDetector{err: wantErr}
	// Text with an AWS key triggers the prefilter, which then calls the
	// stub which returns an error.
	_, err := scanWithDetectorPrefilter(
		"aws_key=AKIAIOSFODNN7EXAMPLE",
		stub, 0.5, 4.0,
	)
	if err == nil || !errors.Is(err, wantErr) {
		t.Errorf("expected error to wrap %v, got %v", wantErr, err)
	}
	if stub.calls == 0 {
		t.Errorf("expected detector.Detect to be called when candidates exist")
	}
}
