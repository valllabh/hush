package scanner_test

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	_ "github.com/valllabh/hush/pkg/bundled"
	"github.com/valllabh/hush/pkg/extractor"
	"github.com/valllabh/hush/pkg/native"
	"github.com/valllabh/hush/pkg/scanner"
)

// expectedSpan mirrors a span in <name>.expected.json.
type expectedSpan struct {
	Start int    `json:"start"`
	End   int    `json:"end"`
	Type  string `json:"type"` // "secret" or "pii"
	Label string `json:"label"`
}

type expectedFile struct {
	Spans []expectedSpan `json:"spans"`
	Notes string         `json:"notes,omitempty"`
}

// classify maps a Finding.Rule to {"secret","pii","unknown"}.
// v1 rules are looked up against extractor.ActiveRules(); v2 emits class
// labels directly ("secret","pii","noise").
func classify(rule string, ruleTypes map[string]string) string {
	r := strings.ToLower(rule)
	switch r {
	case "secret", "pii":
		return r
	case "noise":
		return "noise"
	}
	if t, ok := ruleTypes[rule]; ok {
		return t
	}
	// Some extractor outputs use "high_entropy" — treat as secret class.
	if r == "high_entropy" {
		return "secret"
	}
	return "unknown"
}

func overlaps(aStart, aEnd, bStart, bEnd int) bool {
	if aEnd <= bStart || bEnd <= aStart {
		return false
	}
	return true
}

type counts struct {
	tp, fp, fn int
}

func (c counts) precision() float64 {
	if c.tp+c.fp == 0 {
		return 1.0
	}
	return float64(c.tp) / float64(c.tp+c.fp)
}

func (c counts) recall() float64 {
	if c.tp+c.fn == 0 {
		return 1.0
	}
	return float64(c.tp) / float64(c.tp+c.fn)
}

// scoreFile classifies findings against expected spans for one file and
// returns per-class TP/FP/FN counts. A finding counts as TP only when it
// overlaps an expected span AND has the same class. Class mismatches on
// overlap count as FP for the wrong class (and the expected span is still
// owed a TP from a correct-class finding, else FN).
func scoreFile(findings []scanner.Finding, expected []expectedSpan, ruleTypes map[string]string) (secret, pii counts, notes []string) {
	// Track which expected spans were satisfied (per class).
	expHit := make([]bool, len(expected))

	for _, f := range findings {
		cls := classify(f.Rule, ruleTypes)
		if cls != "secret" && cls != "pii" {
			// noise/unknown findings - ignore (treat as no-flag)
			continue
		}
		matched := false
		classMismatch := false
		for i, e := range expected {
			if !overlaps(f.Start, f.End, e.Start, e.End) {
				continue
			}
			if e.Type == cls {
				matched = true
				expHit[i] = true
				break
			}
			classMismatch = true
		}
		if matched {
			if cls == "secret" {
				secret.tp++
			} else {
				pii.tp++
			}
		} else {
			if classMismatch {
				notes = append(notes, fmt.Sprintf("class-mismatch finding rule=%s cls=%s span=%q", f.Rule, cls, f.Span))
			}
			if cls == "secret" {
				secret.fp++
			} else {
				pii.fp++
			}
		}
	}

	for i, e := range expected {
		if expHit[i] {
			continue
		}
		if e.Type == "secret" {
			secret.fn++
		} else if e.Type == "pii" {
			pii.fn++
		}
	}
	return
}

func loadExpected(t *testing.T, p string) expectedFile {
	t.Helper()
	b, err := os.ReadFile(p)
	if err != nil {
		t.Fatalf("read %s: %v", p, err)
	}
	var ef expectedFile
	if err := json.Unmarshal(b, &ef); err != nil {
		t.Fatalf("parse %s: %v", p, err)
	}
	return ef
}

// buildRuleTypeMap returns rule-name -> "secret"/"pii" using the live
// extractor rule set so the harness stays in sync with rules.json.
func buildRuleTypeMap() map[string]string {
	m := map[string]string{}
	for _, r := range extractor.ActiveRules() {
		m[r.Name] = r.Type
	}
	return m
}

func TestCorpus_V1_vs_V2(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping corpus benchmark in -short mode")
	}

	corpusRoot := filepath.Join("testdata", "corpus")
	if _, err := os.Stat(corpusRoot); err != nil {
		t.Skipf("corpus missing: %v", err)
	}

	// Walk to discover .txt cases.
	type fileCase struct {
		path     string
		text     string
		expected expectedFile
		group    string // top-level subdir name
	}
	var cases []fileCase
	err := filepath.WalkDir(corpusRoot, func(p string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() || !strings.HasSuffix(p, ".txt") {
			return nil
		}
		expPath := strings.TrimSuffix(p, ".txt") + ".expected.json"
		ef := loadExpected(t, expPath)
		raw, err := os.ReadFile(p)
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(corpusRoot, p)
		group := strings.SplitN(filepath.ToSlash(rel), "/", 2)[0]
		cases = append(cases, fileCase{path: p, text: string(raw), expected: ef, group: group})
		return nil
	})
	if err != nil {
		t.Fatalf("walk: %v", err)
	}
	sort.Slice(cases, func(i, j int) bool { return cases[i].path < cases[j].path })
	if len(cases) == 0 {
		t.Fatal("no corpus files found")
	}

	// Build v1 scanner (regex + bundled sequence classifier).
	s1, err := scanner.New(scanner.Options{})
	if err != nil {
		t.Fatalf("v1 scanner.New: %v", err)
	}
	defer s1.Close()

	// Build v2 scanner (regex prefilter + bundled NER detector). Skip if
	// the embedded NER assets are missing (build tag / asset config issue).
	det, err := native.NewBundledDetector()
	if err != nil {
		t.Skipf("v2 bundled detector unavailable: %v", err)
	}
	defer det.Close()
	s2, err := scanner.New(scanner.Options{ModelOff: true, DetectorPrefilter: true, MinConfidence: 0.5})
	if err != nil {
		t.Fatalf("v2 scanner.New: %v", err)
	}
	defer s2.Close()
	s2.UseDetector(detectorAdapter{d: det})

	ruleTypes := buildRuleTypeMap()

	type rowResult struct {
		name         string
		group        string
		v1Sec, v1Pii counts
		v2Sec, v2Pii counts
		v1Findings   []scanner.Finding
		v2Findings   []scanner.Finding
	}
	var rows []rowResult
	var v1SecAgg, v1PiiAgg, v2SecAgg, v2PiiAgg counts

	for _, c := range cases {
		f1, err := s1.ScanString(c.text)
		if err != nil {
			t.Errorf("v1 ScanString %s: %v", c.path, err)
			continue
		}
		f2, err := s2.ScanString(c.text)
		if err != nil {
			t.Errorf("v2 ScanString %s: %v", c.path, err)
			continue
		}
		v1S, v1P, _ := scoreFile(f1, c.expected.Spans, ruleTypes)
		v2S, v2P, _ := scoreFile(f2, c.expected.Spans, ruleTypes)

		v1SecAgg.tp += v1S.tp
		v1SecAgg.fp += v1S.fp
		v1SecAgg.fn += v1S.fn
		v1PiiAgg.tp += v1P.tp
		v1PiiAgg.fp += v1P.fp
		v1PiiAgg.fn += v1P.fn
		v2SecAgg.tp += v2S.tp
		v2SecAgg.fp += v2S.fp
		v2SecAgg.fn += v2S.fn
		v2PiiAgg.tp += v2P.tp
		v2PiiAgg.fp += v2P.fp
		v2PiiAgg.fn += v2P.fn

		rel, _ := filepath.Rel(corpusRoot, c.path)
		rows = append(rows, rowResult{
			name: rel, group: c.group,
			v1Sec: v1S, v1Pii: v1P, v2Sec: v2S, v2Pii: v2P,
			v1Findings: f1, v2Findings: f2,
		})
	}

	// Per-file table.
	var b strings.Builder
	fmt.Fprintf(&b, "\n=== Per-file results (TP/FP/FN, sec | pii) ===\n")
	fmt.Fprintf(&b, "%-44s | %-22s | %-22s\n", "file", "v1 sec / v1 pii", "v2 sec / v2 pii")
	fmt.Fprintf(&b, "%s\n", strings.Repeat("-", 96))
	for _, r := range rows {
		v1c := fmt.Sprintf("%d/%d/%d %d/%d/%d", r.v1Sec.tp, r.v1Sec.fp, r.v1Sec.fn, r.v1Pii.tp, r.v1Pii.fp, r.v1Pii.fn)
		v2c := fmt.Sprintf("%d/%d/%d %d/%d/%d", r.v2Sec.tp, r.v2Sec.fp, r.v2Sec.fn, r.v2Pii.tp, r.v2Pii.fp, r.v2Pii.fn)
		fmt.Fprintf(&b, "%-44s | %-22s | %-22s\n", r.name, v1c, v2c)
	}

	fmt.Fprintf(&b, "\n=== Aggregate ===\n")
	fmt.Fprintf(&b, "v1 secret: P=%.3f R=%.3f (TP=%d FP=%d FN=%d)\n", v1SecAgg.precision(), v1SecAgg.recall(), v1SecAgg.tp, v1SecAgg.fp, v1SecAgg.fn)
	fmt.Fprintf(&b, "v1 pii:    P=%.3f R=%.3f (TP=%d FP=%d FN=%d)\n", v1PiiAgg.precision(), v1PiiAgg.recall(), v1PiiAgg.tp, v1PiiAgg.fp, v1PiiAgg.fn)
	fmt.Fprintf(&b, "v2 secret: P=%.3f R=%.3f (TP=%d FP=%d FN=%d)\n", v2SecAgg.precision(), v2SecAgg.recall(), v2SecAgg.tp, v2SecAgg.fp, v2SecAgg.fn)
	fmt.Fprintf(&b, "v2 pii:    P=%.3f R=%.3f (TP=%d FP=%d FN=%d)\n", v2PiiAgg.precision(), v2PiiAgg.recall(), v2PiiAgg.tp, v2PiiAgg.fp, v2PiiAgg.fn)

	// Detailed dump of every finding for visibility.
	fmt.Fprintf(&b, "\n=== Findings detail ===\n")
	for _, r := range rows {
		fmt.Fprintf(&b, "\n-- %s\n", r.name)
		fmt.Fprintf(&b, "  v1: ")
		if len(r.v1Findings) == 0 {
			fmt.Fprintf(&b, "(none)\n")
		} else {
			fmt.Fprintf(&b, "\n")
			for _, f := range r.v1Findings {
				fmt.Fprintf(&b, "    rule=%s start=%d end=%d conf=%.2f span=%q\n", f.Rule, f.Start, f.End, f.Confidence, f.Span)
			}
		}
		fmt.Fprintf(&b, "  v2: ")
		if len(r.v2Findings) == 0 {
			fmt.Fprintf(&b, "(none)\n")
		} else {
			fmt.Fprintf(&b, "\n")
			for _, f := range r.v2Findings {
				fmt.Fprintf(&b, "    rule=%s start=%d end=%d conf=%.2f span=%q\n", f.Rule, f.Start, f.End, f.Confidence, f.Span)
			}
		}
	}

	t.Log(b.String())

	// v2 thresholds — be honest, do not lower these to make CI green.
	const (
		minSecretP = 0.85
		minSecretR = 0.80
		minPiiP    = 0.80
		minPiiR    = 0.70
	)
	if v2SecAgg.precision() < minSecretP {
		t.Errorf("v2 secret precision %.3f below threshold %.2f", v2SecAgg.precision(), minSecretP)
	}
	if v2SecAgg.recall() < minSecretR {
		t.Errorf("v2 secret recall %.3f below threshold %.2f", v2SecAgg.recall(), minSecretR)
	}
	if v2PiiAgg.precision() < minPiiP {
		t.Errorf("v2 pii precision %.3f below threshold %.2f", v2PiiAgg.precision(), minPiiP)
	}
	if v2PiiAgg.recall() < minPiiR {
		t.Errorf("v2 pii recall %.3f below threshold %.2f", v2PiiAgg.recall(), minPiiR)
	}
}
