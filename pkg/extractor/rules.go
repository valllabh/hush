// Package extractor holds the regex+entropy candidate finder.
//
// Rules are stored in internal/extractor/rules.json (embedded via //go:embed).
// Mirror the Python extractor at src/bmsd/extractor/rules.py.
//
// At runtime, users can customise via --rules-file <path>. The JSON schema is:
//
//	{
//	  "rules":    [ {"name": "internal_tok", "pattern": "\\bmytok_[a-z0-9]{20}\\b"} ],
//	  "disabled": ["aws_access_key_id"]
//	}
//
// A bare array of rule objects is also accepted as a shorthand.
package extractor

import (
	_ "embed"
	"encoding/json"
	"fmt"
	"regexp"
)

// Rule is the compiled representation of one regex pattern.
type Rule struct {
	Name  string
	Regex *regexp.Regexp
	// ValueGroup is the capture group index that holds the secret. 0 = whole match.
	ValueGroup int
	// Type is "secret" or "pii". Defaults to "secret" when unset.
	Type string
}

// RuleJSON is the on-disk / flag form of a rule.
type RuleJSON struct {
	Name       string `json:"name"`
	Type       string `json:"type,omitempty"`
	Pattern    string `json:"pattern"`
	ValueGroup int    `json:"value_group,omitempty"`
}

// Rule type constants.
const (
	RuleTypeSecret = "secret"
	RuleTypePII    = "pii"
)

// RulesConfig is the top-level schema accepted by LoadRulesJSON.
type RulesConfig struct {
	Rules    []RuleJSON `json:"rules"`
	Disabled []string   `json:"disabled"`
}

//go:embed rules.json
var embeddedRulesJSON []byte

// Rules is the default rule set, parsed from embeddedRulesJSON at init().
var Rules []Rule

var activeRules []Rule

// DefaultRulesJSON returns the raw embedded JSON; used by `hush rules --json`.
func DefaultRulesJSON() []byte { return embeddedRulesJSON }

// ActiveRules returns the rule set Extract currently uses.
func ActiveRules() []Rule {
	if len(activeRules) == 0 {
		return Rules
	}
	return activeRules
}

// FilterActiveRulesByTypes narrows the active rule set to the given types
// (e.g. "secret", "pii"). Empty or nil allows all. Unknown types are ignored.
func FilterActiveRulesByTypes(types []string) {
	if len(types) == 0 {
		return
	}
	allow := map[string]bool{}
	for _, t := range types {
		allow[t] = true
	}
	src := ActiveRules()
	out := make([]Rule, 0, len(src))
	for _, r := range src {
		if allow[r.Type] {
			out = append(out, r)
		}
	}
	activeRules = out
}

// BuildActiveRules sets (defaults minus disabled) plus extras as active.
func BuildActiveRules(extras []Rule, disabled []string) {
	dis := map[string]bool{}
	for _, d := range disabled {
		dis[d] = true
	}
	extraByName := map[string]Rule{}
	for _, r := range extras {
		extraByName[r.Name] = r
	}
	out := make([]Rule, 0, len(Rules)+len(extras))
	for _, r := range Rules {
		if dis[r.Name] {
			continue
		}
		if override, ok := extraByName[r.Name]; ok {
			out = append(out, override)
			delete(extraByName, r.Name)
			continue
		}
		out = append(out, r)
	}
	for _, r := range extras {
		if _, pending := extraByName[r.Name]; pending {
			out = append(out, r)
		}
	}
	activeRules = out
}

// LoadRulesJSON parses a JSON blob and applies the combined rule set.
func LoadRulesJSON(data []byte) error {
	cfg, err := parseRulesConfig(data)
	if err != nil {
		return err
	}
	extras, err := compileRules(cfg.Rules)
	if err != nil {
		return err
	}
	BuildActiveRules(extras, cfg.Disabled)
	return nil
}

func compileRules(specs []RuleJSON) ([]Rule, error) {
	out := make([]Rule, 0, len(specs))
	for _, rj := range specs {
		if rj.Name == "" || rj.Pattern == "" {
			return nil, fmt.Errorf("rule must have name and pattern: %+v", rj)
		}
		re, err := regexp.Compile(rj.Pattern)
		if err != nil {
			return nil, fmt.Errorf("rule %q: %w", rj.Name, err)
		}
		vg := rj.ValueGroup
		if vg == 0 && re.NumSubexp() >= 1 {
			vg = 1
		}
		ty := rj.Type
		if ty == "" {
			ty = RuleTypeSecret
		}
		out = append(out, Rule{Name: rj.Name, Regex: re, ValueGroup: vg, Type: ty})
	}
	return out, nil
}

func parseRulesConfig(data []byte) (RulesConfig, error) {
	var cfg RulesConfig
	if err := json.Unmarshal(data, &cfg); err == nil && (len(cfg.Rules) > 0 || len(cfg.Disabled) > 0) {
		return cfg, nil
	}
	var arr []RuleJSON
	if err := json.Unmarshal(data, &arr); err == nil {
		return RulesConfig{Rules: arr}, nil
	}
	return cfg, fmt.Errorf("rules JSON: expected object {rules,disabled} or bare array")
}

func init() {
	var cfg struct {
		Rules []RuleJSON `json:"rules"`
	}
	if err := json.Unmarshal(embeddedRulesJSON, &cfg); err != nil {
		panic(fmt.Sprintf("extractor: parse embedded rules.json: %v", err))
	}
	rules, err := compileRules(cfg.Rules)
	if err != nil {
		panic(fmt.Sprintf("extractor: compile embedded rules: %v", err))
	}
	Rules = rules
}
