package extractor

import (
	"math"
	"strings"
	"testing"
)

func TestShannonEntropy(t *testing.T) {
	tests := []struct {
		name string
		in   string
		min  float64
		max  float64
	}{
		{"empty", "", 0, 0},
		{"single_char", "aaaaaaaa", 0, 0.01},
		{"uniform_hex", "0123456789abcdef", 3.9, 4.1},
		{"high_entropy_b64", "wJalrXUtnFEMI/K7MDENG/bPxRfiCYEXAMPLEKEY", 4.4, 5.0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ShannonEntropy(tt.in)
			if got < tt.min || got > tt.max {
				t.Errorf("ShannonEntropy(%q) = %v, want [%v, %v]", tt.in, got, tt.min, tt.max)
			}
		})
	}
}

func TestRuleMatchAWS(t *testing.T) {
	text := "AWS_ACCESS_KEY=AKIAIOSFODNN7EXAMPLE\nend"
	cands := Extract(text, 64, 4.0)
	if len(cands) == 0 {
		t.Fatal("expected AWS key to be extracted")
	}
	var awsFound bool
	for _, c := range cands {
		if c.SourceRule == "aws_access_key_id" && c.Span == "AKIAIOSFODNN7EXAMPLE" {
			awsFound = true
		}
	}
	if !awsFound {
		t.Fatalf("aws_access_key_id not matched; got: %+v", cands)
	}
}

func TestRuleMatchGitHubPAT(t *testing.T) {
	text := `token: "ghp_abcdefghijklmnopqrstuvwxyz0123456789"`
	cands := Extract(text, 64, 4.0)
	for _, c := range cands {
		if c.SourceRule == "github_pat" && strings.HasPrefix(c.Span, "ghp_") {
			return
		}
	}
	t.Fatalf("github_pat not matched; got: %+v", cands)
}

func TestRuleMatchJWT(t *testing.T) {
	jwt := "eyJhbGciOiJIUzI1NiJ9.eyJzdWIiOiIxMjM0NTY3ODkwIn0.SflKxwRJSMeKKF2QT4fwpMeJf36POk6yJV_adQssw5c"
	cands := Extract("Authorization: Bearer "+jwt, 64, 4.0)
	for _, c := range cands {
		if c.SourceRule == "jwt" && c.Span == jwt {
			return
		}
	}
	t.Fatalf("jwt not matched; got: %+v", cands)
}

func TestRuleMatchGenericAssignment(t *testing.T) {
	text := `password = "Tx7!pQz9@mBk2$vLw"`
	cands := Extract(text, 64, 4.0)
	if len(cands) == 0 {
		t.Fatal("expected generic_assignment_secret to match")
	}
	// First candidate should be the value, not the full "password=..." match.
	if got := cands[0].Span; got != "Tx7!pQz9@mBk2$vLw" {
		t.Errorf("capture group value not used: got %q, want %q", got, "Tx7!pQz9@mBk2$vLw")
	}
}

func TestDedupePrefersRulesOverEntropy(t *testing.T) {
	text := "api_key=AKIAIOSFODNN7EXAMPLE"
	cands := Extract(text, 64, 3.0)
	for _, c := range cands {
		if c.Span == "AKIAIOSFODNN7EXAMPLE" && c.SourceRule != "aws_access_key_id" {
			t.Errorf("rule hit lost to entropy on overlap: got %s", c.SourceRule)
		}
	}
}

func TestRedact(t *testing.T) {
	tests := []struct {
		in, want string
	}{
		{"abc", "***"},
		{"abcdef", "******"},
		{"abcdefg", "abc*efg"},
		{"sk-proj-verylongsecretvalue", "sk-*********************lue"},
	}
	for _, tt := range tests {
		got := Redact(tt.in)
		if got != tt.want {
			t.Errorf("Redact(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
}

func TestCommitHashEntropyBelowThreshold(t *testing.T) {
	// Full hex sha1 has entropy ~3.7-4.0, near our 4.0 threshold. At 4.0 it
	// may or may not trigger; at threshold 4.2 it must not.
	text := "commit 4f3a8b2c9d1e5f7a0b3c2d4e6f8a9b1c by alice"
	cands := Extract(text, 64, 4.2)
	for _, c := range cands {
		if c.SourceRule == "high_entropy" && strings.Contains(c.Span, "4f3a8b2c") {
			t.Errorf("commit sha shouldn't trip entropy at threshold 4.2: %+v", c)
		}
	}
}

// assertClose lets us keep entropy asserts tidy.
func assertClose(t *testing.T, got, want, tol float64) {
	t.Helper()
	if math.Abs(got-want) > tol {
		t.Errorf("got %v, want %v (±%v)", got, want, tol)
	}
}
