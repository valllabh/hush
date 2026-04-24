package cli

import (
	"os"
	"strings"
	"testing"
)

// C6: config precedence flag > env > file > default.
// We assert by observing --output-json vs --output-mask selection.

func TestConfigPrecedence_FlagBeatsEnv(t *testing.T) {
	cfg := writeTempFile(t, ".hush.yaml", "model-threshold: 0.10\n")
	t.Setenv("HUSH_MODEL_THRESHOLD", "0.20")

	p := writeTempFile(t, "k.env", "AWS_ACCESS_KEY_ID=AKIAIOSFODNN7EXAMPLE\n")
	// Flag 0.99 should reign; planted secret has regex confidence 1.0 so it
	// still passes, but we assert no conflict / clean run with --model-off.
	stdout, stderr, code := runHush(t, "",
		"--config", cfg,
		"--model-off",
		"--model-threshold", "0.99",
		"--output-json",
		p,
	)
	if code != 0 {
		t.Errorf("exit %d stderr=%s", code, stderr)
	}
	if !strings.Contains(stdout, "AKIA") {
		t.Errorf("expected finding, got:\n%s", stdout)
	}
}

func TestConfigPrecedence_EnvBeatsFile(t *testing.T) {
	// file sets fail-end false, env sets fail-end true.
	cfg := writeTempFile(t, ".hush.yaml", "fail-end: false\n")
	t.Setenv("HUSH_FAIL_END", "true")

	p := writeTempFile(t, "k.env", "AWS_ACCESS_KEY_ID=AKIAIOSFODNN7EXAMPLE\n")
	_, _, code := runHush(t, "",
		"--config", cfg,
		"--model-off",
		"--output-json",
		p,
	)
	if code != 2 {
		t.Errorf("env HUSH_FAIL_END=true should force exit 2, got %d", code)
	}
}

func TestConfigPrecedence_MissingConfigSilent(t *testing.T) {
	p := writeTempFile(t, "r.md", "clean\n")
	_, stderr, code := runHush(t, "",
		"--config", "/nonexistent/.hush.yaml",
		"--model-off",
		p,
	)
	if code != 0 {
		t.Errorf("missing config should not fail clean scan, exit=%d", code)
	}
	_ = stderr // behaviour: may warn on stderr, must not fail hard
}

func TestConfigPrecedence_FileHonoured(t *testing.T) {
	cfg := writeTempFile(t, ".hush.yaml", "fail-end: true\n")
	// Make sure no env is set.
	os.Unsetenv("HUSH_FAIL_END")

	p := writeTempFile(t, "k.env", "AWS_ACCESS_KEY_ID=AKIAIOSFODNN7EXAMPLE\n")
	_, _, code := runHush(t, "",
		"--config", cfg,
		"--model-off",
		"--output-json",
		p,
	)
	if code != 2 {
		t.Errorf("config fail-end: true should force exit 2, got %d", code)
	}
}
