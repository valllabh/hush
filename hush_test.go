package hush_test

import (
	"strings"
	"testing"

	"github.com/valllabh/hush"
)

func TestTopLevel_AliasesMatchScanner(t *testing.T) {
	// Compile time proof the aliases are interchangeable.
	var _ hush.Options = hush.Options{}
	var _ *hush.Scanner
	var _ hush.Finding
	if hush.ModelVersion == "" {
		t.Fatal("ModelVersion empty")
	}
}

func TestTopLevel_NewModelOff(t *testing.T) {
	s, err := hush.New(hush.Options{ModelOff: true})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer s.Close()

	findings, err := s.ScanString(`api_key="ghp_abcdefghijklmnopqrstuvwxyz0123456789"`)
	if err != nil {
		t.Fatal(err)
	}
	if len(findings) == 0 {
		t.Fatal("expected findings")
	}
}

func TestTopLevel_Redact(t *testing.T) {
	s, err := hush.New(hush.Options{ModelOff: true})
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()

	text := `token="ghp_abcdefghijklmnopqrstuvwxyz0123456789"`
	masked, findings, err := s.Redact(text, "[REDACTED:%s]")
	if err != nil {
		t.Fatal(err)
	}
	if len(findings) == 0 {
		t.Fatal("no findings")
	}
	if strings.Contains(masked, "ghp_abcdefghijklmnopqrstuvwxyz0123456789") {
		t.Errorf("secret leaked: %q", masked)
	}
}

func TestTopLevel_DefaultSmoke(t *testing.T) {
	// Default uses the embedded classifier; skip gracefully when ORT is
	// unavailable on the host (same behaviour as classifier tests).
	s, err := hush.Default()
	if err != nil {
		if strings.Contains(err.Error(), "onnxruntime") {
			t.Skipf("onnxruntime unavailable: %v", err)
		}
		t.Fatalf("Default: %v", err)
	}
	defer s.Close()

	findings, err := s.ScanString(`export AWS_ACCESS_KEY_ID=AKIAIOSFODNN7EXAMPLE`)
	if err != nil {
		t.Fatalf("scan: %v", err)
	}
	// With the embedded model at default threshold we expect at least one
	// finding on a planted AWS key.
	if len(findings) == 0 {
		t.Error("expected at least one finding on planted AWS key")
	}
}
