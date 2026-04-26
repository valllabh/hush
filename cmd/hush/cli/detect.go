package cli

import (
	"encoding/json"
	"fmt"
	"io"
	"os"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	// Blank import wires scanner.DefaultScorerFactory to the embedded
	// classifier so the v1 fallback path can score candidates.
	_ "github.com/valllabh/hush/pkg/bundled"
	"github.com/valllabh/hush/pkg/native"
	"github.com/valllabh/hush/pkg/scanner"
)

// NewDetectCmd implements the v2 NER subcommand:
//
//	hush detect [files...]
//
// Behaviour:
//   - With --model/--tokenizer pointing at a v2 NER hbin, runs the
//     Detector end-to-end and emits findings as JSON lines.
//   - Without flags (or when the supplied model is v1 / sequence
//     classification), falls back to the existing regex + scorer pipeline
//     using the embedded classifier so users with v1 builds still get
//     useful output. A one-line stderr note is printed in fallback mode.
//   - With no positional args, reads stdin as a single document
//     (file=<stdin>).
func NewDetectCmd() *cobra.Command {
	var modelPath, tokPath string
	var noPrefilter bool
	cmd := &cobra.Command{
		Use:   "detect [files...]",
		Short: "Run the v2 NER detector (auto-falls back to v1 regex+scorer).",
		RunE: func(c *cobra.Command, args []string) error {
			n, err := runDetect(args, modelPath, tokPath, !noPrefilter)
			if err != nil {
				return err
			}
			if n > 0 {
				// Match `hush scan --fail-on-finding` semantics: non-zero
				// exit when findings are reported, so detect drops straight
				// into CI and pre-commit hooks. SilenceUsage prevents cobra
				// from printing the help banner on a normal "found things"
				// exit.
				c.SilenceUsage = true
				c.SilenceErrors = true
				os.Exit(1)
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&modelPath, "model", "", "Path to a v2 token-classification .hbin (default: use embedded v1 fallback)")
	cmd.Flags().StringVar(&tokPath, "tokenizer", "", "Path to tokenizer.json paired with --model")
	cmd.Flags().BoolVar(&noPrefilter, "no-prefilter", false, "Disable regex+entropy prefilter and run the model on every window (slower, max recall)")
	cmd.Flags().Bool("output-reveal-secrets", false, "DANGEROUS: include the raw secret/PII value (`span` field) in JSON output. Default emits only the redacted preview.")
	_ = viper.BindPFlag("output-reveal-secrets", cmd.Flags().Lookup("output-reveal-secrets"))
	return cmd
}

// detectorAdapter bridges *native.Detector to scanner.Detector so the
// scanner package never has to import pkg/native (avoids an import cycle).
type detectorAdapter struct{ d *native.Detector }

func (a detectorAdapter) Detect(text string) ([]scanner.DetectedSpan, error) {
	spans, err := a.d.Detect(text)
	if err != nil {
		return nil, err
	}
	out := make([]scanner.DetectedSpan, len(spans))
	for i, s := range spans {
		out[i] = scanner.DetectedSpan{Start: s.Start, End: s.End, Type: s.Type, Score: s.Score}
	}
	return out, nil
}

func runDetect(paths []string, modelPath, tokPath string, prefilter bool) (int, error) {
	var (
		s   *scanner.Scanner
		err error
	)

	useV2Flag := modelPath != "" && tokPath != ""
	useV2Embedded := false
	if !useV2Flag {
		// Auto-detect from the embedded v2 asset. If the embedded model
		// is token-classification, prefer it; otherwise fall through to
		// the v1 regex+scorer path.
		if meta, merr := native.EmbeddedV2Meta(); merr == nil && meta.IsTokenClassification() {
			useV2Embedded = true
		}
	}

	v2Opts := scanner.Options{ModelOff: true, DetectorPrefilter: prefilter}

	switch {
	case useV2Flag:
		det, derr := native.LoadDetector(modelPath, tokPath)
		if derr != nil {
			return 0, fmt.Errorf("loading detector: %w", derr)
		}
		s, err = scanner.New(v2Opts)
		if err != nil {
			return 0, err
		}
		s.UseDetector(detectorAdapter{d: det})
	case useV2Embedded:
		det, derr := native.NewBundledDetector()
		if derr != nil {
			return 0, fmt.Errorf("loading bundled v2 detector: %w", derr)
		}
		s, err = scanner.New(v2Opts)
		if err != nil {
			return 0, err
		}
		s.UseDetector(detectorAdapter{d: det})
	default:
		// v1 mode: embedded sequence classifier + regex extractor.
		fmt.Fprintln(os.Stderr, "hush detect: v1 mode (regex + embedded sequence classifier); pass --model/--tokenizer for v2 NER")
		s, err = scanner.New(scanner.Options{})
		if err != nil {
			return 0, err
		}
	}
	defer s.Close()

	enc := json.NewEncoder(os.Stdout)
	reveal := viper.GetBool("output-reveal-secrets")
	total := 0

	emit := func(f scanner.Finding) {
		if reveal {
			_ = enc.Encode(scanner.RevealedFinding{F: f})
		} else {
			_ = enc.Encode(f)
		}
	}

	if len(paths) == 0 {
		b, rerr := io.ReadAll(os.Stdin)
		if rerr != nil {
			return 0, fmt.Errorf("reading stdin: %w", rerr)
		}
		findings, scanErr := s.ScanString(string(b))
		if scanErr != nil {
			return 0, scanErr
		}
		for i := range findings {
			findings[i].File = "<stdin>"
			emit(findings[i])
		}
		return len(findings), nil
	}

	for _, p := range paths {
		data, rerr := os.ReadFile(p)
		if rerr != nil {
			fmt.Fprintf(os.Stderr, "hush detect: read %s: %v\n", p, rerr)
			continue
		}
		findings, scanErr := s.ScanString(string(data))
		if scanErr != nil {
			fmt.Fprintf(os.Stderr, "hush detect: scan %s: %v\n", p, scanErr)
			continue
		}
		for i := range findings {
			findings[i].File = p
			emit(findings[i])
		}
		total += len(findings)
	}
	return total, nil
}
