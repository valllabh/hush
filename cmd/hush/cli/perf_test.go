package cli

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// Real-world CLI performance tests against a generated fixture tree. The
// numbers we care about: startup cost, throughput (files/sec), and peak RSS.
// Gates are generous — these are perf-observability tests, not hard floors.

// Skip these unless explicitly asked; they take ~20-40s total.
func perfEnabled() bool { return os.Getenv("HUSH_PERF") != "" }

// generateTree creates a directory with n source files mixing clean and
// dirty content. Returns the path.
func generateTree(t *testing.T, n int) string {
	t.Helper()
	dir := t.TempDir()
	for i := 0; i < n; i++ {
		name := fmt.Sprintf("file_%04d.py", i)
		path := filepath.Join(dir, name)
		var content string
		switch i % 10 {
		case 0:
			content = "def f():\n    AWS_ACCESS_KEY_ID = \"AKIAIOSFODNN7EXAMPLE\"\n" +
				"    return AWS_ACCESS_KEY_ID\n"
		case 1:
			content = "import os\nAPI_KEY = os.environ['API_KEY']  # ghp_" +
				"abcdefghijklmnopqrstuvwxyz0123456789\n"
		default:
			// boring clean file
			content = strings.Repeat("# just a normal comment\ndef g(): pass\n", 20)
		}
		if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return dir
}

// runAndMeasure executes the hush binary with the given args and reports
// wall time and stderr-derived resource usage via /usr/bin/time -l (macOS)
// or /usr/bin/time -v (linux). Falls back to a plain run when time isn't
// available.
type runStats struct {
	Wall      time.Duration
	MaxRSSkib int64  // peak resident memory in KiB
	Stdout    string
	ExitCode  int
	Err       error
}

func timeCmdArgs() (string, []string) {
	// macOS: /usr/bin/time -l prints rusage (maximum resident set size in bytes)
	// linux: /usr/bin/time -v prints "Maximum resident set size (kbytes)"
	if _, err := os.Stat("/usr/bin/time"); err == nil {
		switch os := strings.ToLower(os_Name()); {
		case strings.Contains(os, "darwin"):
			return "/usr/bin/time", []string{"-l"}
		default:
			return "/usr/bin/time", []string{"-v"}
		}
	}
	return "", nil
}

func os_Name() string {
	b, _ := exec.Command("uname", "-s").Output()
	return strings.TrimSpace(string(b))
}

func runHushMeasured(t *testing.T, args ...string) runStats {
	t.Helper()
	if hushBin == "" {
		t.Skip("hush binary not available")
	}
	timeBin, timeArgs := timeCmdArgs()
	var cmd *exec.Cmd
	if timeBin != "" {
		full := append(append([]string(nil), timeArgs...), append([]string{hushBin}, args...)...)
		cmd = exec.Command(timeBin, full...)
	} else {
		cmd = exec.Command(hushBin, args...)
	}
	var out, errb bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &errb
	start := time.Now()
	err := cmd.Run()
	wall := time.Since(start)
	s := runStats{Wall: wall, Stdout: out.String(), Err: err}
	if ee, ok := err.(*exec.ExitError); ok {
		s.ExitCode = ee.ExitCode()
	}
	s.MaxRSSkib = parseMaxRSS(errb.String())
	return s
}

// parseMaxRSS extracts the maximum resident set size from /usr/bin/time output.
// macOS reports bytes with a " maximum resident set size" suffix; linux
// reports kilobytes on a "Maximum resident set size (kbytes): N" line.
func parseMaxRSS(s string) int64 {
	for _, line := range strings.Split(s, "\n") {
		line = strings.TrimSpace(line)
		if strings.Contains(line, "maximum resident set size") && !strings.Contains(line, "kbytes") {
			// macOS: "<bytes>  maximum resident set size"
			parts := strings.Fields(line)
			if len(parts) >= 1 {
				var n int64
				fmt.Sscanf(parts[0], "%d", &n)
				return n / 1024
			}
		}
		if strings.Contains(line, "Maximum resident set size (kbytes)") {
			parts := strings.Split(line, ":")
			if len(parts) == 2 {
				var n int64
				fmt.Sscanf(strings.TrimSpace(parts[1]), "%d", &n)
				return n
			}
		}
	}
	return 0
}

// ---- the actual perf tests ----

func TestPerf_StartupCost(t *testing.T) {
	if !perfEnabled() {
		t.Skip("set HUSH_PERF=1 to run perf tests")
	}
	s := runHushMeasured(t, "version")
	if s.Err != nil && s.ExitCode != 0 {
		t.Fatalf("version failed: %v\n%s", s.Err, s.Stdout)
	}
	t.Logf("startup (version): wall=%s maxRSS=%dKiB", s.Wall, s.MaxRSSkib)
	if s.Wall > 2*time.Second {
		t.Errorf("startup too slow: %s (budget 2s)", s.Wall)
	}
}

func TestPerf_ScanTree_100Files_ModelOff(t *testing.T) {
	if !perfEnabled() {
		t.Skip("set HUSH_PERF=1 to run perf tests")
	}
	dir := generateTree(t, 100)
	s := runHushMeasured(t, "--model-off", "--output-json", dir)
	if s.Err != nil && s.ExitCode != 0 {
		t.Fatalf("scan failed: %v\n%s", s.Err, s.Stdout)
	}
	throughput := float64(100) / s.Wall.Seconds()
	t.Logf("scan 100 files model-off: wall=%s  throughput=%.0f files/s  maxRSS=%dKiB",
		s.Wall, throughput, s.MaxRSSkib)
	// Sanity: 100 files should take under 5s with model off.
	if s.Wall > 5*time.Second {
		t.Errorf("too slow: %s (budget 5s)", s.Wall)
	}
}

func TestPerf_ScanTree_100Files_WithModel(t *testing.T) {
	if !perfEnabled() {
		t.Skip("set HUSH_PERF=1 to run perf tests")
	}
	dir := generateTree(t, 100)
	s := runHushMeasured(t, "--output-json", dir)
	if s.Err != nil && s.ExitCode != 0 {
		t.Fatalf("scan failed: %v\n%s", s.Err, s.Stdout)
	}
	throughput := float64(100) / s.Wall.Seconds()
	t.Logf("scan 100 files with-model: wall=%s  throughput=%.0f files/s  maxRSS=%dKiB",
		s.Wall, throughput, s.MaxRSSkib)
}

func TestPerf_ScanTree_1000Files_ModelOff(t *testing.T) {
	if !perfEnabled() {
		t.Skip("set HUSH_PERF=1 to run perf tests")
	}
	dir := generateTree(t, 1000)
	s := runHushMeasured(t, "--model-off", "--output-json", dir)
	if s.Err != nil && s.ExitCode != 0 {
		t.Fatalf("scan failed: %v\n%s", s.Err, s.Stdout)
	}
	throughput := float64(1000) / s.Wall.Seconds()
	t.Logf("scan 1000 files model-off: wall=%s  throughput=%.0f files/s  maxRSS=%dKiB",
		s.Wall, throughput, s.MaxRSSkib)
}

func TestPerf_ScanFixture(t *testing.T) {
	if !perfEnabled() {
		t.Skip("set HUSH_PERF=1 to run perf tests")
	}
	// Use the training fixture if available.
	fixture := filepath.Join("..", "..", "..", "..", "training", "tests", "fixtures", "repo")
	if _, err := os.Stat(fixture); err != nil {
		t.Skipf("training fixture not present: %v", err)
	}
	for _, modelOff := range []bool{true, false} {
		args := []string{"--output-json", fixture}
		if modelOff {
			args = append([]string{"--model-off"}, args...)
		}
		label := "with-model"
		if modelOff {
			label = "model-off"
		}
		s := runHushMeasured(t, args...)
		t.Logf("fixture %s: wall=%s maxRSS=%dKiB exitCode=%d", label, s.Wall, s.MaxRSSkib, s.ExitCode)
	}
}
