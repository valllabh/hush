package cli

import (
	"github.com/spf13/pflag"
)

// flagAlias maps OLD (deprecated) flag names to NEW (namespaced) names. The
// old ones stay registered (hidden) so existing scripts keep working.
var flagAlias = map[string]string{
	"mask":              "output-mask",
	"json":              "output-json",
	"out":               "output-file",
	"placeholder":       "output-placeholder",
	"in-place":          "", // removed entirely
	"mask-suffix":       "", // removed entirely
	"no-model":          "model-off",
	"threshold":         "model-threshold",
	"entropy-threshold": "extract-entropy",
	"ctx":               "extract-ctx",
	"fail-on-finding":   "fail-end",
	"workers":           "perf-workers",
	"max-file-mb":       "file-max-mb",
	"ext":               "file-include", // semantic widening
	"skip-ext":          "file-exclude", // semantic widening
	"skip-dir":          "file-exclude",
	"skip-path":         "file-exclude",
	"skip-pattern":      "file-exclude",
	"include-pattern":   "file-include",
	"rules-file":        "rule-file",
}

// addAllFlags installs the namespaced flag set plus hidden back-compat aliases.
func addAllFlags(f *pflag.FlagSet) {
	// --- Files to scan: file-* ---
	f.StringSlice("file-include", nil, "Paths/names/patterns/extensions to keep (repeatable, comma)")
	f.StringSlice("file-exclude", nil, "Paths/names/patterns/extensions to skip (repeatable, comma)")
	f.Int64("file-max-mb", 10, "Skip files larger than this many MB")

	// --- Rules: rule-* ---
	f.StringArray("rule-add", nil, "Inline rule: NAME=PATTERN (repeatable)")
	f.String("rule-file", "", "JSON rules file (see `hush rules --json` for schema)")
	f.StringSlice("rule-include", nil, "Only use these rule names (allowlist)")
	f.StringSlice("rule-exclude", nil, "Disable these rule names (blocklist)")
	f.StringSlice("detect", []string{"both"}, "Which rule types to run: secrets, pii, both (comma-separated)")

	// --- Model: model-* ---
	f.Bool("model-off", false, "Skip ML filter; use regex+entropy only (faster, more FPs)")
	f.Float64("model-threshold", 0.5, "Model confidence threshold")

	// --- Extractor: extract-* ---
	f.Float64("extract-entropy", 4.0, "Shannon entropy threshold")
	f.Int("extract-ctx", 256, "Context window chars")

	// --- Output: output-* ---
	f.BoolP("output-mask", "m", false, "Output masked text (stdin/piped input only)")
	f.BoolP("output-json", "j", false, "Output findings as NDJSON (default)")
	f.StringP("output-file", "o", "", "Redirect output to file")
	f.StringP("output-placeholder", "p", "", "Fixed placeholder (default: [REDACTED_<RULE>_<N>])")

	// --- Failure: fail-* ---
	f.BoolP("fail-end", "f", false, "Exit 2 at end if any finding (scan completes)")
	f.Bool("fail-fast", false, "Exit 2 as soon as the first finding is seen")

	// --- Performance: perf-* ---
	f.IntP("perf-workers", "w", defaultWorkers(), "Parallel workers (default: NumCPU-2)")

	// --- Back-compat hidden aliases ---
	// These keep old scripts working. All are hidden from `--help`.
	f.Bool("mask", false, "(deprecated: use --output-mask)")
	f.Bool("json", false, "(deprecated: use --output-json)")
	f.String("out", "", "(deprecated: use --output-file)")
	f.String("placeholder", "", "(deprecated: use --output-placeholder)")
	f.Bool("no-model", false, "(deprecated: use --model-off)")
	f.Float64("threshold", 0.5, "(deprecated: use --model-threshold)")
	f.Float64("entropy-threshold", 4.0, "(deprecated: use --extract-entropy)")
	f.Int("ctx", 256, "(deprecated: use --extract-ctx)")
	f.Bool("fail-on-finding", false, "(deprecated: use --fail-end)")
	f.Int("workers", defaultWorkers(), "(deprecated: use --perf-workers)")
	f.Int64("max-file-mb", 10, "(deprecated: use --file-max-mb)")
	f.StringSlice("ext", nil, "(deprecated: use --file-include)")
	f.StringSlice("skip-ext", nil, "(deprecated: use --file-exclude)")
	f.StringSlice("skip-dir", nil, "(deprecated: use --file-exclude)")
	f.StringSlice("skip-path", nil, "(deprecated: use --file-exclude)")
	f.StringSlice("skip-pattern", nil, "(deprecated: use --file-exclude)")
	f.StringSlice("include-pattern", nil, "(deprecated: use --file-include)")
	f.String("rules-file", "", "(deprecated: use --rule-file)")

	for old := range flagAlias {
		_ = f.MarkHidden(old)
	}
}
