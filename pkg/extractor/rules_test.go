package extractor

import (
	"strings"
	"testing"
)

// Reset state between tests since BuildActiveRules mutates a package-level var.
func reset(t *testing.T) {
	t.Helper()
	activeRules = nil
}

func TestDefaultRulesLoaded(t *testing.T) {
	if len(Rules) < 5 {
		t.Fatalf("expected built-in rules loaded from rules.json, got %d", len(Rules))
	}
	found := map[string]bool{}
	for _, r := range Rules {
		found[r.Name] = true
	}
	for _, want := range []string{"aws_access_key_id", "github_pat", "jwt", "private_key_pem"} {
		if !found[want] {
			t.Errorf("missing default rule %q", want)
		}
	}
}

func TestDefaultRulesJSONDumpRoundTrip(t *testing.T) {
	defer reset(t)
	data := DefaultRulesJSON()
	if !strings.Contains(string(data), `"aws_access_key_id"`) {
		t.Fatal("expected embedded JSON to contain aws_access_key_id")
	}
	// Loading the dump should produce a rule set with the same names.
	if err := LoadRulesJSON(data); err != nil {
		t.Fatalf("LoadRulesJSON: %v", err)
	}
	if len(ActiveRules()) != len(Rules) {
		t.Errorf("round-trip loses rules: got %d, want %d", len(ActiveRules()), len(Rules))
	}
}

func TestLoadRulesJSON_AddsCustomRule(t *testing.T) {
	defer reset(t)
	cfg := `{
	  "rules": [
	    {"name": "internal_tok", "pattern": "\\bmytok_[a-z0-9]{20}\\b"}
	  ]
	}`
	if err := LoadRulesJSON([]byte(cfg)); err != nil {
		t.Fatalf("LoadRulesJSON: %v", err)
	}
	active := ActiveRules()
	var hasCustom, hasAWS bool
	for _, r := range active {
		if r.Name == "internal_tok" {
			hasCustom = true
		}
		if r.Name == "aws_access_key_id" {
			hasAWS = true
		}
	}
	if !hasCustom {
		t.Error("custom rule not added")
	}
	if !hasAWS {
		t.Error("default rule lost when extending")
	}
}

func TestLoadRulesJSON_DisablesDefault(t *testing.T) {
	defer reset(t)
	cfg := `{"disabled": ["aws_access_key_id"]}`
	if err := LoadRulesJSON([]byte(cfg)); err != nil {
		t.Fatalf("LoadRulesJSON: %v", err)
	}
	for _, r := range ActiveRules() {
		if r.Name == "aws_access_key_id" {
			t.Error("disabled rule still active")
		}
	}
}

func TestLoadRulesJSON_BareArrayShorthand(t *testing.T) {
	defer reset(t)
	cfg := `[{"name": "my_rule", "pattern": "abc123"}]`
	if err := LoadRulesJSON([]byte(cfg)); err != nil {
		t.Fatalf("LoadRulesJSON: %v", err)
	}
	for _, r := range ActiveRules() {
		if r.Name == "my_rule" {
			return
		}
	}
	t.Error("bare array rule not loaded")
}

func TestLoadRulesJSON_OverrideByName(t *testing.T) {
	defer reset(t)
	cfg := `{"rules":[{"name":"aws_access_key_id","pattern":"XXX[0-9]{3}"}]}`
	if err := LoadRulesJSON([]byte(cfg)); err != nil {
		t.Fatalf("LoadRulesJSON: %v", err)
	}
	count := 0
	for _, r := range ActiveRules() {
		if r.Name == "aws_access_key_id" {
			count++
			if !strings.Contains(r.Regex.String(), "XXX") {
				t.Errorf("override didn't take effect: %s", r.Regex.String())
			}
		}
	}
	if count != 1 {
		t.Errorf("expected exactly one rule with name aws_access_key_id, got %d", count)
	}
}

func TestLoadRulesJSON_BadPatternReturnsError(t *testing.T) {
	defer reset(t)
	cfg := `{"rules":[{"name":"bad","pattern":"(unclosed"}]}`
	if err := LoadRulesJSON([]byte(cfg)); err == nil {
		t.Error("expected error for bad regex")
	}
}

func TestLoadRulesJSON_MissingFieldError(t *testing.T) {
	defer reset(t)
	cfg := `{"rules":[{"name":""}]}`
	if err := LoadRulesJSON([]byte(cfg)); err == nil {
		t.Error("expected error for empty name")
	}
}
