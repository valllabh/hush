package extractor

import (
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

// Regression test for plan item #5: a base64-encoded AWS access key
// hidden inside a string literal must be discovered by the encoded-pass
// even when the raw regex misses it.
func TestExtract_EncodedSecret_AWSBase64(t *testing.T) {
	// "AKIAIOSFODNN7EXAMPLE" -> base64
	const enc = "QUtJQUlPU0ZPRE5ON0VYQU1QTEU="
	text := "config_blob = \"" + enc + "\""
	cands := Extract(text, 64, 4.0)
	found := false
	for _, c := range cands {
		if strings.HasPrefix(c.SourceRule, "encoded_aws_access_key_id") {
			found = true
		}
	}
	if !found {
		t.Errorf("encoded AWS key missed; cands=%+v", cands)
	}
}

// Regression test for plan item #9: an inline base64 data URI for an
// image must NOT trigger entropy false positives (its body is masked
// before entropy scoring).
func TestExtract_EmbeddedDataURI_NoEntropyFalseAlarms(t *testing.T) {
	// Construct a long alphanumeric pseudo-image body well above the
	// entropy threshold.
	body := strings.Repeat("AbCdEfGhIjKlMnOpQrStUvWxYz0123456789", 12)
	text := "<img src=\"data:image/png;base64," + body + "\">"
	cands := Extract(text, 64, 4.0)
	for _, c := range cands {
		if c.SourceRule == "high_entropy" {
			t.Errorf("data: URI body emitted entropy hit (#9 regression): %q", c.Span)
		}
	}
}

// Regression test for plan item #9: a CERTIFICATE PEM block's body must
// not appear as a high-entropy candidate. The x509_certificate rule may
// still match the boundary (intentional).
func TestExtract_CertificateBody_NoEntropyFalseAlarms(t *testing.T) {
	body := strings.Repeat("AbCdEfGhIjKlMnOpQrStUvWxYz0123456789\n", 8)
	text := "-----BEGIN CERTIFICATE-----\n" + body + "-----END CERTIFICATE-----"
	cands := Extract(text, 64, 4.0)
	for _, c := range cands {
		if c.SourceRule == "high_entropy" {
			t.Errorf("CERTIFICATE body emitted entropy hit (#9 regression): %q", c.Span)
		}
	}
}
