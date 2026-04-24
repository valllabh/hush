package scanner

import (
	"strings"
	"testing"
)

func newOff(t *testing.T) *Scanner {
	t.Helper()
	s, err := New(Options{ModelOff: true})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	return s
}

func TestOptionsDefaults(t *testing.T) {
	got := Options{}.withDefaults()
	if got.MinConfidence != 0.5 {
		t.Errorf("MinConfidence default = %v", got.MinConfidence)
	}
	if got.EntropyThreshold != 4.0 {
		t.Errorf("EntropyThreshold default = %v", got.EntropyThreshold)
	}
	if got.CtxChars != 64 {
		t.Errorf("CtxChars default = %v", got.CtxChars)
	}
}

func TestNewModelOffNeedsNoFactory(t *testing.T) {
	save := DefaultScorerFactory
	DefaultScorerFactory = nil
	defer func() { DefaultScorerFactory = save }()

	s, err := New(Options{ModelOff: true})
	if err != nil {
		t.Fatalf("ModelOff should not need factory: %v", err)
	}
	defer s.Close()
}

func TestNewWithoutFactoryFails(t *testing.T) {
	save := DefaultScorerFactory
	DefaultScorerFactory = nil
	defer func() { DefaultScorerFactory = save }()

	_, err := New(Options{})
	if err == nil {
		t.Fatal("expected error when no factory and ModelOff=false")
	}
}

func TestScanStringFindsSecret(t *testing.T) {
	s := newOff(t)
	defer s.Close()
	findings, err := s.ScanString(`api_key="ghp_abcdefghijklmnopqrstuvwxyz0123456789"`)
	if err != nil {
		t.Fatal(err)
	}
	if len(findings) == 0 {
		t.Fatal("expected at least one finding")
	}
}

func TestScanReader(t *testing.T) {
	s := newOff(t)
	defer s.Close()
	r := strings.NewReader(`token: ghp_abcdefghijklmnopqrstuvwxyz0123456789`)
	findings, err := s.ScanReader(r)
	if err != nil {
		t.Fatal(err)
	}
	if len(findings) == 0 {
		t.Fatal("expected findings from reader")
	}
}

func TestRedactFixedPlaceholder(t *testing.T) {
	s := newOff(t)
	defer s.Close()
	text := `token="ghp_abcdefghijklmnopqrstuvwxyz0123456789"`
	masked, findings, err := s.Redact(text, "[REDACTED]")
	if err != nil {
		t.Fatal(err)
	}
	if len(findings) == 0 {
		t.Fatal("expected findings")
	}
	if strings.Contains(masked, "ghp_abcdefghijklmnopqrstuvwxyz0123456789") {
		t.Errorf("secret leaked through mask: %q", masked)
	}
	if !strings.Contains(masked, "[REDACTED]") {
		t.Errorf("placeholder missing: %q", masked)
	}
}

func TestRedactRuleFormat(t *testing.T) {
	s := newOff(t)
	defer s.Close()
	text := `token="ghp_abcdefghijklmnopqrstuvwxyz0123456789"`
	masked, findings, err := s.Redact(text, "[REDACTED:%s]")
	if err != nil {
		t.Fatal(err)
	}
	if len(findings) == 0 {
		t.Fatal("expected findings")
	}
	expected := "[REDACTED:" + findings[0].Rule + "]"
	if !strings.Contains(masked, expected) {
		t.Errorf("expected %q in masked output: %q", expected, masked)
	}
}

func TestRedactCleanTextUnchanged(t *testing.T) {
	s := newOff(t)
	defer s.Close()
	clean := "the quick brown fox jumps over the lazy dog"
	masked, findings, err := s.Redact(clean, "[X]")
	if err != nil {
		t.Fatal(err)
	}
	if len(findings) != 0 {
		t.Errorf("unexpected findings in clean text: %v", findings)
	}
	if masked != clean {
		t.Errorf("clean text changed: %q", masked)
	}
}

func TestCloseIdempotent(t *testing.T) {
	s := newOff(t)
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	if err := s.Close(); err != nil {
		t.Errorf("second Close should be no op, got %v", err)
	}
}

func TestNilScannerClose(t *testing.T) {
	var s *Scanner
	if err := s.Close(); err != nil {
		t.Errorf("nil Close should be no op, got %v", err)
	}
}
