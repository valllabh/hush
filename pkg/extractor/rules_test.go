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

// findRule returns the first default Rule whose name matches n.
func findRule(t *testing.T, n string) Rule {
	t.Helper()
	for _, r := range Rules {
		if r.Name == n {
			return r
		}
	}
	t.Fatalf("rule %q not found in defaults", n)
	return Rule{}
}

// matchRule returns true if rule n matches input s.
func matchRule(t *testing.T, n, s string) bool {
	t.Helper()
	return findRule(t, n).Regex.MatchString(s)
}

// TestRuleTypeFieldDefaults asserts every default rule has a type field set
// (either "secret" or "pii"), and that the default-when-missing is "secret".
func TestRuleTypeFieldDefaults(t *testing.T) {
	defer reset(t)
	for _, r := range Rules {
		if r.Type != RuleTypeSecret && r.Type != RuleTypePII {
			t.Errorf("rule %q has invalid type %q", r.Name, r.Type)
		}
	}
	// missing type in JSON should default to secret.
	cfg := `{"rules":[{"name":"x","pattern":"abc"}]}`
	if err := LoadRulesJSON([]byte(cfg)); err != nil {
		t.Fatal(err)
	}
	for _, r := range ActiveRules() {
		if r.Name == "x" && r.Type != RuleTypeSecret {
			t.Errorf("missing type defaulted to %q, want %q", r.Type, RuleTypeSecret)
		}
	}
}

func TestFilterActiveRulesByTypes(t *testing.T) {
	defer reset(t)
	BuildActiveRules(nil, nil)
	FilterActiveRulesByTypes([]string{RuleTypePII})
	for _, r := range ActiveRules() {
		if r.Type != RuleTypePII {
			t.Errorf("rule %q leaked through pii filter (type=%q)", r.Name, r.Type)
		}
	}
	if len(ActiveRules()) == 0 {
		t.Error("expected at least one pii rule")
	}
}

// --- Secret family smoke tests ---

func TestSecretFamilies_Positive(t *testing.T) {
	cases := map[string]string{
		"aws_access_key_id":               "AKIAIOSFODNN7EXAMPLE",
		"aws_secret_access_key":           `aws-secret-key = "wJalrXUtnFEMIK7MDENGbPxRfiCYEXAMPLEKEYab"`,
		"gcp_oauth_client_secret":         "GOCSPX-abcdefghijklmnopqrstuvwxyz12",
		"openai_api_key":                  "sk-proj-aAbBcCdDeEfFgGhHiIjJkKlLmMnNoOpP",
		"anthropic_api_key":               "sk-ant-api03-AAAAAAAAAAAAAAAAAAAA",
		"huggingface_token":               "hf_aBcDeFgHiJkLmNoPqRsTuVwXyZ0123456789",
		"slack_webhook_url":               "https://hooks.slack.com" + "/services/T12345678/B12345678/abcdefghijklmnopqrstuvwx",
		"discord_webhook_url":             "https://discord.com/api/webhooks/123456789012345678/" + "aBcDeFgHiJkLmNoPqRsTuVwXyZ0123456789aBcDeFgHiJkLmNoPqRsTuVwXy",
		"telegram_bot_token":              "123456789:AAEhBP0av28gr8pW7nqOqG8hS9vJ2kLmNop",
		"stripe_publishable_key":          "pk_live_abcdef1234567890ABCDEF",
		"stripe_secret":                   "sk_test_abcdef1234567890ABCDEF",
		"twilio_api_key":                  "SK" + "0123456789abcdef0123456789abcdef",
		"sendgrid_api_key":                "SG" + ".abcdefghijklmnopqrstuv.abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQ",
		"gitlab_pat":                      "glpat-aBcDeFgHiJkLmNoPqRsT",
		"shopify_access_token":            "shpat" + "_abcdef0123456789ABCDEF0123456789",
		"npm_token":                       "npm_abcdefghijklmnopqrstuvwxyz0123456789",
		"pypi_token":                      "pypi-AgEIcHlwaS5vcmcCJGZmZmZmZmZmLWZmZmZmZmZmZmZmLWZmZmZmZmZmZmZmZmZmZmZmZg",
		"dockerhub_pat":                   "dckr_pat_abcdefghijklmnopqrstuvwxyz123",
		"digitalocean_pat":                "dop_v1_" + strings.Repeat("a", 64),
		"azure_storage_connection_string": "DefaultEndpointsProtocol=https;AccountName=foo;AccountKey=" + strings.Repeat("A", 70) + "==",
		"db_uri_creds":                    "mongodb://user:pass123@localhost:27017/db",
		"http_basic_auth_url":             "https://admin:s3cret@example.com/path",
		"x509_certificate":                "-----BEGIN CERTIFICATE-----",
		"pgp_private_key":                 "-----BEGIN PGP PRIVATE KEY BLOCK-----",
		"putty_private_key":               "PuTTY-User-Key-File-2: ssh-rsa",
		"jwt":                             "eyJhbGciOiJIUzI1NiIs.eyJzdWIiOiIxMjM0NTY3ODkwIn0.SflKxwRJSMeKKF2QT4fwpMeJf36POk6yJV_adQssw5c",
	}
	for name, input := range cases {
		if !matchRule(t, name, input) {
			t.Errorf("%s: expected match for %q", name, input)
		}
	}
}

func TestSecretFamilies_Negative(t *testing.T) {
	neg := map[string]string{
		"aws_access_key_id":   "NOTAKEY12345",
		"openai_api_key":      "hello world",
		"anthropic_api_key":   "sk-notant-xxx",
		"huggingface_token":   "hf_short",
		"slack_webhook_url":   "https://hooks.slack.com/x/y/z",
		"stripe_secret":       "sk_foo_abc",
		"db_uri_creds":        "mongodb://localhost:27017",
		"http_basic_auth_url": "https://example.com/no-auth",
		"x509_certificate":    "CERTIFICATE",
		"jwt":                 "eyJfoo",
		"private_key_pem":     "just some text",
	}
	for name, input := range neg {
		if matchRule(t, name, input) {
			t.Errorf("%s: unexpected match for %q", name, input)
		}
	}
}

// --- PII family smoke tests ---

func TestPIIFamilies_Positive(t *testing.T) {
	cases := map[string]string{
		"email_address":          "alice.smith+work@example.co.uk",
		"ipv4_address":           "192.168.1.100",
		"ipv6_address":           "2001:0db8:85a3:0000:0000:8a2e:0370:7334",
		"mac_address":            "00:1A:2B:3C:4D:5E",
		"phone_e164":             "+14155552671",
		"phone_us":               "(415) 555-2671",
		"ssn_us":                 "123-45-6789",
		"sin_ca":                 "123-456-789",
		"nin_uk":                 "AB123456C",
		"aadhaar_in":             "2345 6789 0123",
		"pan_in":                 "ABCDE1234F",
		"cpf_br":                 "123.456.789-09",
		"credit_card_visa":       "4111111111111111",
		"credit_card_mastercard": "5555555555554444",
		"credit_card_amex":       "378282246310005",
		"credit_card_discover":   "6011111111111117",
		"credit_card_jcb":        "3530111333300000",
		"iban":                   "DE89370400440532013000",
		"ein_us":                 "12-3456789",
		"itin_us":                "912-71-1234",
		"passport_in":            "A1234567",
		"drivers_license_us":     "driver's license: D1234567",
	}
	for name, input := range cases {
		if !matchRule(t, name, input) {
			t.Errorf("%s: expected match for %q", name, input)
		}
	}
}

func TestPIIFamilies_Negative(t *testing.T) {
	neg := map[string]string{
		"email_address":    "not-an-email",
		"ipv4_address":     "999.999.999.999",
		"phone_e164":       "12345",
		"ssn_us":           "12-345-678",
		"pan_in":           "abcde1234f",
		"credit_card_visa": "1234567890",
		"iban":             "foo",
		"cpf_br":           "12345678909",
	}
	for name, input := range neg {
		if matchRule(t, name, input) {
			t.Errorf("%s: unexpected match for %q", name, input)
		}
	}
}
