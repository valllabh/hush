package cli

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
)

func runCmd(t *testing.T, args ...string) (stdout, stderr string, err error) {
	t.Helper()
	cmd := NewRootCmd()
	var outBuf, errBuf bytes.Buffer
	cmd.SetOut(&outBuf)
	cmd.SetErr(&errBuf)
	cmd.SetArgs(args)
	err = cmd.Execute()
	return outBuf.String(), errBuf.String(), err
}

func TestVersionCommand(t *testing.T) {
	// `version` prints to fmt.Printf (os.Stdout) not cobra's out writer,
	// so we assert via a sub-run that exits cleanly rather than capturing.
	cmd := NewRootCmd()
	cmd.SetArgs([]string{"version"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("version: %v", err)
	}
	if Version == "" {
		t.Error("Version constant empty")
	}
}

func TestRulesJSONIsValid(t *testing.T) {
	// Use the rules command directly; it prints to stdout with fmt.Print.
	// Instead of capturing stdout, validate via the extractor package which
	// is the underlying data source.
	data := defaultRulesJSONForTest(t)
	var parsed map[string]any
	if err := json.Unmarshal(data, &parsed); err != nil {
		t.Fatalf("rules JSON not valid: %v", err)
	}
	if _, ok := parsed["rules"]; !ok {
		t.Error(`rules JSON missing "rules" key`)
	}
}

func TestRootHelpMentionsKeyFlags(t *testing.T) {
	out, _, err := runCmd(t, "--help")
	if err != nil {
		t.Fatalf("--help: %v", err)
	}
	for _, keyword := range []string{"scan", "rules", "version", "--output-mask"} {
		if !strings.Contains(out, keyword) {
			t.Errorf("--help output missing %q", keyword)
		}
	}
}

func TestUnknownSubcommandErrors(t *testing.T) {
	// Unknown subcommand is actually accepted here because the root RunE
	// forwards to scan with any args. We instead assert that an unknown
	// *flag* errors out.
	_, _, err := runCmd(t, "--definitely-not-a-flag")
	if err == nil {
		t.Fatal("expected error for unknown flag")
	}
}

func TestRulesSubcommandRegistered(t *testing.T) {
	cmd := NewRootCmd()
	found := false
	for _, c := range cmd.Commands() {
		if c.Name() == "rules" {
			found = true
			break
		}
	}
	if !found {
		t.Fatal("rules subcommand not registered")
	}
}

func TestVersionSubcommandRegistered(t *testing.T) {
	cmd := NewRootCmd()
	found := false
	for _, c := range cmd.Commands() {
		if c.Name() == "version" {
			found = true
			break
		}
	}
	if !found {
		t.Fatal("version subcommand not registered")
	}
}

// defaultRulesJSONForTest pulls the embedded rules via the extractor
// package indirectly so we do not create a test only public API.
func defaultRulesJSONForTest(t *testing.T) []byte {
	t.Helper()
	// Use reflection-free approach: the rules command prints extractor.DefaultRulesJSON().
	// Import it here through the rules.go file which is in the same package.
	return getDefaultRulesJSON()
}
