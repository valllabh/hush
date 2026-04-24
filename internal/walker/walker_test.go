package walker

import (
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
)

// makeTree creates:
//
//	root/
//	  a.py
//	  b.log
//	  node_modules/x.js
//	  dump/huge.txt
//	  nested/
//	    secret.env
//	    .angular/cache/chunk.js
func makeTree(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	files := []string{
		"a.py", "b.log",
		"node_modules/x.js",
		"dump/huge.txt",
		"nested/secret.env",
		"nested/.angular/cache/chunk.js",
		"normal/file.go",
	}
	for _, f := range files {
		p := filepath.Join(root, f)
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte("x"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return root
}

func collect(t *testing.T, roots []string, opts Options) []string {
	t.Helper()
	out := make(chan string, 32)
	errs := make(chan error, 32)
	go Walk(roots, opts, out, errs)
	var got []string
	for p := range out {
		got = append(got, p)
	}
	// Drain non-fatal errs.
	select {
	default:
	case <-errs:
	}
	sort.Strings(got)
	return got
}

func TestWalk_DefaultSkipsNodeModulesAndAngular(t *testing.T) {
	root := makeTree(t)
	got := collect(t, []string{root}, DefaultOptions())
	for _, p := range got {
		if strings.Contains(p, "/node_modules/") {
			t.Errorf("node_modules should be skipped: %s", p)
		}
		if strings.Contains(p, "/.angular/") {
			t.Errorf(".angular should be skipped: %s", p)
		}
	}
}

func TestWalk_SkipPath(t *testing.T) {
	root := makeTree(t)
	opts := DefaultOptions()
	opts.SkipPaths = []string{filepath.Join(root, "dump")}
	got := collect(t, []string{root}, opts)
	for _, p := range got {
		if strings.Contains(p, "/dump/") {
			t.Errorf("skip-path failed: %s", p)
		}
	}
}

func TestWalk_SkipGlob(t *testing.T) {
	root := makeTree(t)
	opts := DefaultOptions()
	opts.SkipPatterns = []string{"**/nested/**"}
	got := collect(t, []string{root}, opts)
	for _, p := range got {
		if strings.Contains(p, "/nested/") {
			t.Errorf("skip-glob failed: %s", p)
		}
	}
}

func TestWalk_IncludeGlob(t *testing.T) {
	root := makeTree(t)
	opts := DefaultOptions()
	opts.IncludePatterns = []string{"**/*.py"}
	got := collect(t, []string{root}, opts)
	if len(got) == 0 {
		t.Fatal("include-glob: expected at least 1 match")
	}
	for _, p := range got {
		if !strings.HasSuffix(p, ".py") {
			t.Errorf("include-glob leaked non-py file: %s", p)
		}
	}
}

func TestWalk_OnlyExts(t *testing.T) {
	root := makeTree(t)
	opts := DefaultOptions()
	opts.OnlyExts = ParseExts("py,go")
	got := collect(t, []string{root}, opts)
	for _, p := range got {
		if !(strings.HasSuffix(p, ".py") || strings.HasSuffix(p, ".go")) {
			t.Errorf("only-ext leaked: %s", p)
		}
	}
}

func TestParseExts(t *testing.T) {
	got := ParseExts("py, env, .yaml")
	for _, want := range []string{".py", ".env", ".yaml"} {
		if !got[want] {
			t.Errorf("ParseExts missing %q in %v", want, got)
		}
	}
	if got[""] {
		t.Error("ParseExts should not produce empty entries")
	}
}

func TestNormalisePaths_HomeExpansion(t *testing.T) {
	home, _ := os.UserHomeDir()
	out := NormalisePaths([]string{"~/foo", "/abs/bar", ""})
	if len(out) != 2 {
		t.Fatalf("expected 2 paths, got %v", out)
	}
	if !strings.HasPrefix(out[0], home) {
		t.Errorf("~ not expanded: %s", out[0])
	}
}

func TestClassifyPattern_AbsPath(t *testing.T) {
	opts := DefaultOptions()
	ClassifyPattern("/Users/me/Work/dump", true, &opts)
	if len(opts.SkipPaths) != 1 {
		t.Fatalf("abs path not routed to SkipPaths: %+v", opts.SkipPaths)
	}
}

func TestClassifyPattern_ExtShorthand(t *testing.T) {
	opts := DefaultOptions()
	ClassifyPattern("*.pdf", true, &opts)
	if !opts.SkipExts[".pdf"] {
		t.Fatalf("*.pdf not routed to SkipExts: %+v", opts.SkipExts)
	}
}

func TestClassifyPattern_Wildcard(t *testing.T) {
	opts := DefaultOptions()
	ClassifyPattern("**/cache/**", true, &opts)
	if len(opts.SkipPatterns) != 1 {
		t.Fatalf("**/cache/** not routed to SkipPatterns: %+v", opts.SkipPatterns)
	}
}

func TestClassifyPattern_BareName(t *testing.T) {
	opts := DefaultOptions()
	ClassifyPattern("mydir", true, &opts)
	if !opts.SkipDirs["mydir"] {
		t.Fatalf("bare name not routed to SkipDirs: %+v", opts.SkipDirs)
	}
}

func TestClassifyPattern_IncludeExtEnablesAllowlist(t *testing.T) {
	opts := DefaultOptions()
	ClassifyPattern("*.py", false, &opts)
	if !opts.OnlyExts[".py"] {
		t.Fatalf("include *.py didn't build OnlyExts: %+v", opts.OnlyExts)
	}
}
