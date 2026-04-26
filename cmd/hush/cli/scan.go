package cli

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"runtime"
	"strings"
	"sync"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	"github.com/valllabh/hush/internal/walker"
	// Blank import registers scanner.DefaultScorerFactory. The registered
	// implementation is selected at build time by the pkg/bundled build
	// tags: default builds the ORT path, `-tags=native` builds the pure
	// Go path.
	_ "github.com/valllabh/hush/pkg/bundled"
	"github.com/valllabh/hush/pkg/extractor"
	"github.com/valllabh/hush/pkg/scanner"
)

// NewScanCmd returns the explicit `hush scan` subcommand.
func NewScanCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "scan [paths...]",
		Short: "Scan stdin or paths for secrets (default behaviour).",
		RunE:  func(c *cobra.Command, args []string) error { return runScan(c, args) },
	}
	addAllFlags(cmd.Flags())
	return cmd
}

// bindAllFlagsToViper registers every namespaced flag + every deprecated alias
// so values get merged: flag > env > config > default.
func bindAllFlagsToViper(cmd *cobra.Command) {
	allKeys := []string{
		"file-include", "file-exclude", "file-max-mb",
		"rule-add", "rule-file", "rule-include", "rule-exclude", "detect",
		"model-off", "model-threshold",
		"extract-entropy", "extract-ctx",
		"output-mask", "output-json", "output-file", "output-placeholder",
		"fail-end", "fail-fast",
		"perf-workers",
	}
	for old := range flagAlias {
		allKeys = append(allKeys, old)
	}
	for _, k := range allKeys {
		if pf := cmd.Flags().Lookup(k); pf != nil {
			_ = viper.BindPFlag(k, pf)
		}
	}
}

// aliasValue returns the new-key value if set; otherwise falls back to the
// legacy flag name so deprecated flags still work.
func aliasString(newKey string) string {
	if v := viper.GetString(newKey); v != "" {
		return v
	}
	for old, new_ := range flagAlias {
		if new_ == newKey {
			if v := viper.GetString(old); v != "" {
				return v
			}
		}
	}
	return ""
}

func aliasBool(newKey string) bool {
	if viper.IsSet(newKey) && viper.GetBool(newKey) {
		return true
	}
	for old, new_ := range flagAlias {
		if new_ == newKey && viper.IsSet(old) && viper.GetBool(old) {
			return true
		}
	}
	return false
}

func aliasInt(newKey string, dflt int) int {
	if viper.IsSet(newKey) {
		return viper.GetInt(newKey)
	}
	for old, new_ := range flagAlias {
		if new_ == newKey && viper.IsSet(old) {
			return viper.GetInt(old)
		}
	}
	return dflt
}

func aliasInt64(newKey string, dflt int64) int64 {
	if viper.IsSet(newKey) {
		return viper.GetInt64(newKey)
	}
	for old, new_ := range flagAlias {
		if new_ == newKey && viper.IsSet(old) {
			return viper.GetInt64(old)
		}
	}
	return dflt
}

func aliasFloat(newKey string, dflt float64) float64 {
	if viper.IsSet(newKey) {
		return viper.GetFloat64(newKey)
	}
	for old, new_ := range flagAlias {
		if new_ == newKey && viper.IsSet(old) {
			return viper.GetFloat64(old)
		}
	}
	return dflt
}

// aliasSlice merges any set of the legacy skip-*/include-* into the unified
// file-include / file-exclude buckets.
func aliasSlice(newKey string) []string {
	vals := append([]string{}, viper.GetStringSlice(newKey)...)
	for old, new_ := range flagAlias {
		if new_ == newKey {
			vals = append(vals, viper.GetStringSlice(old)...)
		}
	}
	return vals
}

// runScan is shared by the root command and `hush scan`.
func runScan(cmd *cobra.Command, paths []string) error {
	bindAllFlagsToViper(cmd)

	mask := aliasBool("output-mask")
	asJSON := aliasBool("output-json")
	if !mask && !asJSON {
		asJSON = true
	}

	// Load rules-file first so subsequent extraction uses the merged set.
	if rf := aliasString("rule-file"); rf != "" {
		data, err := os.ReadFile(rf)
		if err != nil {
			return fmt.Errorf("reading rule-file: %w", err)
		}
		if err := extractor.LoadRulesJSON(data); err != nil {
			return fmt.Errorf("loading rule-file: %w", err)
		}
	}
	// Combined --rule-add / --rule-include / --rule-exclude.
	adds := viper.GetStringSlice("rule-add")
	inc := viper.GetStringSlice("rule-include")
	exc := viper.GetStringSlice("rule-exclude")
	if len(adds) > 0 || len(inc) > 0 || len(exc) > 0 {
		if err := applyRuleControl(adds, inc, exc); err != nil {
			return err
		}
	}
	// --detect filters active rules by type (secret / pii).
	if types := resolveDetectTypes(viper.GetStringSlice("detect")); len(types) > 0 {
		// ensure activeRules is populated before filtering (BuildActiveRules
		// with no extras/disabled is a no-op clone of defaults).
		if len(extractor.ActiveRules()) == len(extractor.Rules) && len(adds) == 0 && len(inc) == 0 && len(exc) == 0 {
			extractor.BuildActiveRules(nil, nil)
		}
		extractor.FilterActiveRulesByTypes(types)
	}

	modelOff := aliasBool("model-off")
	threshold := aliasFloat("model-threshold", 0.5)
	entropy := aliasFloat("extract-entropy", 4.0)
	ctx := aliasInt("extract-ctx", 256)
	placeholder := aliasString("output-placeholder")
	outPath := aliasString("output-file")
	failEnd := aliasBool("fail-end")
	failFast := viper.GetBool("fail-fast")
	workers := aliasInt("perf-workers", defaultWorkers())
	maxFileMB := aliasInt64("file-max-mb", 10)

	// Unified include/exclude: auto-detect each entry's kind.
	opts := walker.DefaultOptions()
	opts.MaxFileSize = maxFileMB * 1024 * 1024
	for _, pat := range aliasSlice("file-exclude") {
		for _, part := range strings.Split(pat, ",") {
			walker.ClassifyPattern(part, true, &opts)
		}
	}
	for _, pat := range aliasSlice("file-include") {
		for _, part := range strings.Split(pat, ",") {
			walker.ClassifyPattern(part, false, &opts)
		}
	}

	// Mask + directory input is not a valid combo.
	if mask && len(paths) > 0 {
		for _, p := range paths {
			if info, err := os.Stat(p); err == nil && info.IsDir() {
				return fmt.Errorf("--output-mask is only valid for stdin/piped input, not for directory %q", p)
			}
		}
	}

	// Load classifier once; shared across workers. The concrete backend
	// (ORT vs pure Go native) is selected at build time via pkg/bundled.
	var sc scanner.Scorer
	if !modelOff {
		factory := scanner.DefaultScorerFactory
		if factory == nil {
			return fmt.Errorf("no scorer factory registered (build without pkg/bundled?)")
		}
		s, closer, err := factory(1)
		if err != nil {
			return fmt.Errorf("loading model: %w (tip: try --model-off)", err)
		}
		if closer != nil {
			defer closer()
		}
		sc = s
	}

	// Stdin mode: triggered by no args, or by the conventional `-` path.
	if len(paths) == 0 || (len(paths) == 1 && paths[0] == "-") {
		return runStdin(mask, asJSON, outPath, threshold, entropy, ctx, placeholder, failEnd, sc)
	}

	total, aborted := runMulti(paths, opts, workers, asJSON,
		threshold, entropy, ctx, failFast, sc)

	if (failFast && aborted) || (failEnd && total > 0) {
		return &ExitError{Code: 2}
	}
	return nil
}

// resolveDetectTypes expands the --detect flag values (secrets, pii, both, or
// comma-separated combos) into a list of rule types to keep. Returns nil if
// the filter is a no-op (i.e. "both" or equivalent).
func resolveDetectTypes(raw []string) []string {
	want := map[string]bool{}
	for _, v := range raw {
		for _, part := range strings.Split(v, ",") {
			p := strings.ToLower(strings.TrimSpace(part))
			switch p {
			case "", "both", "all":
				want[extractor.RuleTypeSecret] = true
				want[extractor.RuleTypePII] = true
			case "secret", "secrets":
				want[extractor.RuleTypeSecret] = true
			case "pii":
				want[extractor.RuleTypePII] = true
			}
		}
	}
	if want[extractor.RuleTypeSecret] && want[extractor.RuleTypePII] {
		return nil
	}
	out := make([]string, 0, len(want))
	for k := range want {
		out = append(out, k)
	}
	return out
}

// applyRuleControl builds the effective rule set from inline adds + include +
// exclude, then installs it. Precedence: inline adds override defaults with
// the same name; include (if non-empty) restricts to that allowlist; exclude
// disables those rules in addition.
func applyRuleControl(adds, include, exclude []string) error {
	specs := make([]extractor.RuleJSON, 0, len(adds))
	for _, s := range adds {
		i := strings.IndexByte(s, '=')
		if i <= 0 || i == len(s)-1 {
			return fmt.Errorf("--rule-add %q: expected NAME=PATTERN", s)
		}
		specs = append(specs, extractor.RuleJSON{Name: s[:i], Pattern: s[i+1:]})
	}
	// Compile inline rules via a JSON round-trip (reuses extractor validation).
	var extras []extractor.Rule
	if len(specs) > 0 {
		data, _ := json.Marshal(map[string]any{"rules": specs})
		if err := extractor.LoadRulesJSON(data); err != nil {
			return err
		}
		// Pull them out of the just-set active set so we can recompose with
		// include/exclude correctly.
		existing := map[string]bool{}
		for _, r := range extractor.Rules {
			existing[r.Name] = true
		}
		for _, r := range extractor.ActiveRules() {
			if !existing[r.Name] {
				extras = append(extras, r)
			} else {
				// default name overridden by an inline add -> treat as extra too
				for _, sp := range specs {
					if sp.Name == r.Name {
						extras = append(extras, r)
						break
					}
				}
			}
		}
	}

	excl := append([]string{}, exclude...)
	if len(include) > 0 {
		incSet := map[string]bool{}
		for _, n := range include {
			incSet[strings.TrimSpace(n)] = true
		}
		for _, r := range extractor.Rules {
			if !incSet[r.Name] {
				excl = append(excl, r.Name)
			}
		}
	}
	extractor.BuildActiveRules(extras, excl)
	return nil
}

func runStdin(mask, asJSON bool, outPath string, threshold, entropy float64,
	ctx int, placeholder string, failEnd bool, sc scanner.Scorer) error {
	b, err := io.ReadAll(os.Stdin)
	if err != nil {
		return fmt.Errorf("reading stdin: %w", err)
	}
	text := string(b)
	findings, err := scanner.Scan(text, threshold, entropy, ctx, sc)
	if err != nil {
		return fmt.Errorf("scan: %w", err)
	}
	var out io.Writer = os.Stdout
	if outPath != "" {
		f, err := os.Create(outPath)
		if err != nil {
			return fmt.Errorf("opening %s: %w", outPath, err)
		}
		defer f.Close()
		out = f
	}
	if mask {
		io.WriteString(out, scanner.MaskText(text, findings, placeholder))
	} else if asJSON {
		enc := json.NewEncoder(out)
		enc.SetIndent("", "  ")
		toEmit := findings
		if !viper.GetBool("output-reveal-secrets") {
			toEmit = scanner.SafeForOutput(findings)
		}
		_ = enc.Encode(toEmit)
	}
	if failEnd && len(findings) > 0 {
		return &ExitError{Code: 2}
	}
	return nil
}

// runMulti scans paths in parallel and streams NDJSON findings to stdout.
// Mask output for multi-path input is no longer supported: mask is a
// stream transformation, not a filesystem rewriter.
// largeFileBytes is the size above which we switch from a slurping
// os.ReadFile + Scan path to a chunked ScanReader. Mirrors plan #12.
const largeFileBytes = 50 * 1024 * 1024

// scanFilePath chooses the right scan strategy based on file size:
// small files go through Scan (simple), large files stream via the
// chunked ScanReader so peak RSS stays bounded.
func scanFilePath(path string, threshold, entropy float64, ctx int, sc scanner.Scorer) ([]scanner.Finding, error) {
	info, statErr := os.Stat(path)
	if statErr != nil {
		return nil, statErr
	}
	if info.Size() <= largeFileBytes {
		data, err := os.ReadFile(path)
		if err != nil {
			return nil, err
		}
		return scanner.Scan(string(data), threshold, entropy, ctx, sc)
	}
	// Large file: stream via a Scanner facade so chunking + dedupe is
	// handled centrally. We assemble a fresh Scanner here because the
	// worker holds only the bare scorer.
	s, err := scanner.New(scanner.Options{
		MinConfidence:    threshold,
		EntropyThreshold: entropy,
		CtxChars:         ctx,
		ModelOff:         sc == nil,
	})
	if err != nil {
		return nil, err
	}
	defer s.Close()
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	fmt.Fprintf(os.Stderr, "hush: scan %s: streaming chunked (%.1f MB)\n", path, float64(info.Size())/(1024*1024))
	return s.ScanReader(f)
}

func runMulti(roots []string, opts walker.Options, workers int, asJSON bool,
	threshold, entropy float64, ctx int, failFast bool,
	sc scanner.Scorer) (int, bool) {

	if workers < 1 {
		workers = 1
	}
	runtime.GOMAXPROCS(workers)

	jobs := make(chan string, workers*4)
	errs := make(chan error, 64)
	abort := make(chan struct{})
	var abortOnce sync.Once
	aborted := false
	triggerAbort := func() {
		abortOnce.Do(func() {
			aborted = true
			close(abort)
		})
	}

	go walker.Walk(roots, opts, jobs, errs)
	go func() {
		for e := range errs {
			fmt.Fprintf(os.Stderr, "hush: %v\n", e)
		}
	}()

	var outMu sync.Mutex
	bw := bufio.NewWriter(os.Stdout)
	defer bw.Flush()
	enc := json.NewEncoder(bw)

	var wg sync.WaitGroup
	var tf int64
	var tfMu sync.Mutex

	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for path := range jobs {
				select {
				case <-abort:
					continue
				default:
				}
				findings, err := scanFilePath(path, threshold, entropy, ctx, sc)
				if err != nil {
					fmt.Fprintf(os.Stderr, "hush: scan %s: %v\n", path, err)
					continue
				}
				for j := range findings {
					findings[j].File = path
				}
				if len(findings) == 0 {
					continue
				}
				tfMu.Lock()
				tf += int64(len(findings))
				tfMu.Unlock()

				if failFast {
					triggerAbort()
				}
				if asJSON {
					reveal := viper.GetBool("output-reveal-secrets")
					outMu.Lock()
					for _, f := range findings {
						if reveal {
							_ = enc.Encode(scanner.RevealedFinding{F: f})
						} else {
							_ = enc.Encode(f)
						}
					}
					outMu.Unlock()
				}
			}
		}()
	}

	wg.Wait()
	close(errs)
	return int(tf), aborted
}
