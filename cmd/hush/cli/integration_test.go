package cli

import (
	"bytes"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// hushBin is the path to the freshly compiled hush binary, set by TestMain.
var hushBin string

func TestMain(m *testing.M) {
	tmp, err := os.MkdirTemp("", "hush-bin-*")
	if err != nil {
		panic(err)
	}
	defer os.RemoveAll(tmp)

	hushBin = filepath.Join(tmp, "hush")
	// go build from the parent directory of this cli package, honouring
	// the test binary's build tag so the native and default paths are
	// actually exercised end to end when requested.
	buildArgs := []string{"build"}
	if integrationBuildTags != "" {
		buildArgs = append(buildArgs, "-tags="+integrationBuildTags)
	}
	buildArgs = append(buildArgs, "-o", hushBin, "../")
	build := exec.Command("go", buildArgs...)
	build.Stdout = os.Stderr
	build.Stderr = os.Stderr
	if err := build.Run(); err != nil {
		// If build fails (e.g. ORT not available at build time), mark binary
		// missing; individual tests will skip rather than fail.
		hushBin = ""
	}
	os.Exit(m.Run())
}

// runHush executes the compiled hush binary with args + optional stdin.
// Returns stdout, stderr, and the process' exit code (0 on success).
func runHush(t *testing.T, stdin string, args ...string) (string, string, int) {
	t.Helper()
	if hushBin == "" {
		t.Skip("hush binary not available in this environment")
	}
	cmd := exec.Command(hushBin, args...)
	if stdin != "" {
		cmd.Stdin = strings.NewReader(stdin)
	}
	var out, errb bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &errb
	err := cmd.Run()
	code := 0
	if ee, ok := err.(*exec.ExitError); ok {
		code = ee.ExitCode()
	} else if err != nil {
		t.Fatalf("run: %v", err)
	}
	return out.String(), errb.String(), code
}

// writeTempFile drops content at a new file under t.TempDir().
func writeTempFile(t *testing.T, name, content string) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), name)
	if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	return p
}

// ---------- C1: dirty fixture detects planted secret ----------

func TestIntegration_DirtyRepoDetectsSecret(t *testing.T) {
	dirty := writeTempFile(t, "app.env",
		`AWS_ACCESS_KEY_ID=AKIAIOSFODNN7EXAMPLE`+"\n"+
			`AWS_SECRET_ACCESS_KEY=wJalrXUtnFEMI/K7MDENG/bPxRfiCYEXAMPLEKEY`+"\n")

	start := time.Now()
	stdout, stderr, code := runHush(t, "", "--model-off", "--output-json", dirty)
	dur := time.Since(start)
	if dur > time.Second*5 {
		t.Errorf("scan took too long: %v", dur)
	}
	if code != 0 {
		// Without --fail-end a dirty scan still exits 0 (findings-only in stdout).
		t.Errorf("expected exit 0 without --fail-end, got %d; stderr=%s", code, stderr)
	}
	if !strings.Contains(stdout, "AKIA") && !strings.Contains(stdout, "aws_") {
		t.Errorf("expected a finding in output, got:\n%s", stdout)
	}
}

// ---------- C2: clean fixture produces no findings and exit 0 ----------

func TestIntegration_CleanRepoExitsClean(t *testing.T) {
	clean := writeTempFile(t, "readme.md", "# Hello\n\nThis file has no secrets.\n")
	stdout, stderr, code := runHush(t, "", "--model-off", "--output-json", clean)
	if code != 0 {
		t.Errorf("clean scan should exit 0, got %d; stderr=%s", code, stderr)
	}
	if strings.TrimSpace(stdout) != "" {
		t.Errorf("clean scan should have empty stdout, got %q", stdout)
	}
}

// ---------- C3: NDJSON findings have stable shape ----------

func TestIntegration_NDJSONShape(t *testing.T) {
	p := writeTempFile(t, "k.env", `AWS_ACCESS_KEY_ID=AKIAIOSFODNN7EXAMPLE`+"\n")
	stdout, _, _ := runHush(t, "", "--model-off", "--output-json", p)
	lines := strings.Split(strings.TrimSpace(stdout), "\n")
	if len(lines) == 0 {
		t.Fatal("no NDJSON lines")
	}
	var f map[string]any
	if err := json.Unmarshal([]byte(lines[0]), &f); err != nil {
		t.Fatalf("line is not JSON: %v\n%s", err, lines[0])
	}
	for _, k := range []string{"line", "rule", "redacted"} {
		if _, ok := f[k]; !ok {
			t.Errorf("finding missing key %q: %v", k, f)
		}
	}
}

// ---------- C4: rules --json round trips via --rule-file ----------

func TestIntegration_RulesJSONRoundTrip(t *testing.T) {
	dumpOut, _, code := runHush(t, "", "rules", "--json")
	if code != 0 {
		t.Fatalf("rules --json exit %d", code)
	}
	var parsed map[string]any
	if err := json.Unmarshal([]byte(dumpOut), &parsed); err != nil {
		t.Fatalf("rules --json output is not valid JSON: %v", err)
	}
	if _, ok := parsed["rules"]; !ok {
		t.Error(`output missing "rules" key`)
	}

	// Write back out to disk and feed to --rule-file against a fixture.
	rulesPath := writeTempFile(t, "rules.json", dumpOut)
	fixture := writeTempFile(t, "x.env", `AWS_ACCESS_KEY_ID=AKIAIOSFODNN7EXAMPLE`+"\n")
	stdout, stderr, code := runHush(t, "", "--model-off", "--rule-file", rulesPath, fixture)
	if code != 0 {
		t.Errorf("rule-file load failed: stderr=%s", stderr)
	}
	if !strings.Contains(stdout, "AKIA") && !strings.Contains(stdout, "aws_") {
		t.Errorf("rule-file scan missed planted secret: %s", stdout)
	}
}

// ---------- C5: --model-off runs without libonnxruntime being needed ----------

func TestIntegration_ModelOffSkipsClassifier(t *testing.T) {
	// Poison the env var to an invalid lib path. If the classifier were loaded,
	// hush would fail to initialise ORT.
	os.Setenv("ONNXRUNTIME_LIB", "/nonexistent/path/libonnxruntime.so")
	defer os.Unsetenv("ONNXRUNTIME_LIB")

	p := writeTempFile(t, "k.env", `AWS_ACCESS_KEY_ID=AKIAIOSFODNN7EXAMPLE`+"\n")
	stdout, stderr, code := runHush(t, "", "--model-off", "--output-json", p)
	if code != 0 {
		t.Errorf("--model-off should succeed regardless of ORT availability; exit %d stderr=%s", code, stderr)
	}
	if strings.TrimSpace(stdout) == "" {
		t.Errorf("--model-off should still produce findings from extractor, got empty")
	}
}

// ---------- C6: exit code behaviour ----------

func TestIntegration_FailEndYieldsExit2(t *testing.T) {
	p := writeTempFile(t, "k.env", `AWS_ACCESS_KEY_ID=AKIAIOSFODNN7EXAMPLE`+"\n")
	_, _, code := runHush(t, "", "--model-off", "--output-json", "--fail-end", p)
	if code != 2 {
		t.Errorf("--fail-end with findings: expected exit 2, got %d", code)
	}
}

func TestIntegration_FailEndCleanExit0(t *testing.T) {
	p := writeTempFile(t, "r.md", "no secrets here\n")
	_, _, code := runHush(t, "", "--model-off", "--output-json", "--fail-end", p)
	if code != 0 {
		t.Errorf("--fail-end on clean: expected exit 0, got %d", code)
	}
}
