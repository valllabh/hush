// Package cli defines hush's cobra commands.
package cli

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

const Version = "0.1.11"

// configFile is set by --config / $HUSH_CONFIG; loaded lazily.
var configFile string
var presetName string

// NewRootCmd returns the root command. Running hush without a subcommand
// dispatches to `hush scan` to keep the old ergonomics.
func NewRootCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "hush [paths...]",
		Short: "Portable secrets and PII scrubber (BitNet classifier embedded).",
		Long: `hush detects secrets in text, code, config, and logs.

Pipeline
--------
  1. EXTRACTOR: regex rules + Shannon entropy scan (rules embedded; see
     hush rules --json). Override/extend via --rule-file / --rule-add.
  2. CLASSIFIER: embedded BitNet 1.58-bit student decides secret-or-not.
     Disable with --model-off (faster, more false positives).
  3. OUTPUT: findings NDJSON (--output-json, default) OR masked text
     (--output-mask, stdin only).

Inputs
------
  stdin             echo "..." | hush --output-mask
  paths             hush /Users/me/Work  src/  configs/

Flags are grouped by namespace:

  file-*      which files get scanned
  rule-*      custom/builtin rule management
  model-*     ML classifier control
  extract-*   extractor tuning
  output-*    output mode and destination
  fail-*      exit code behaviour
  perf-*      parallelism

Filtering (file-*)
------------------
  --file-include and --file-exclude accept any of:
    /abs/path           -> path prefix excluded/included
    name                -> directory or file basename anywhere
    *.ext               -> extension allowlist/blocklist
    **/pattern/**       -> wildcard (* = chars, ** = any path incl. /)
  Values are repeatable or comma-separated.

Rules (rule-*)
--------------
  --rule-add NAME=PATTERN    inline add/override (repeatable)
  --rule-file rules.json     bulk JSON (see hush rules --json for schema)
  --rule-include AWS,JWT     allowlist by rule name
  --rule-exclude generic...  disable rule by name

Config file + env
-----------------
  Flags merge from: flag > HUSH_* env var > .hush.yaml (cwd or $HOME) > default.
  Named presets under "presets:" in config, selected via --preset <name>.

Exit codes
----------
  0  no findings, or findings without --fail-* flags
  1  error (unreadable path, bad rules file, etc.)
  2  findings detected with --fail-end or --fail-fast

Examples
--------
  echo 'api_key="sk-..."' | hush --output-mask
  hush /Users/me/Work --file-exclude /Users/me/Work/dump
  hush --model-off --fail-fast ./repo
  hush --file-include '*.py,*.env,*.yml' ./src
  hush --rule-add 'tok=\bmytok_[a-z0-9]{20}\b' ./src
  hush rules --json > rules.json   # bootstrap a custom rule set`,
		SilenceUsage:  true,
		SilenceErrors: true,
		Args:          cobra.ArbitraryArgs,
		// Default action when no subcommand is given: behave like `scan`.
		RunE: func(c *cobra.Command, args []string) error { return runScan(c, args) },
	}

	cmd.PersistentFlags().StringVar(&configFile, "config", "", "Config file (default: .hush.yaml in cwd or $HOME)")
	cmd.PersistentFlags().StringVar(&presetName, "preset", "", "Named preset from config (e.g. strict, fast)")

	// Namespaced flag set plus hidden legacy aliases.
	addAllFlags(cmd.Flags())

	cmd.AddCommand(NewScanCmd(), NewVersionCmd(), NewRulesCmd(), NewDetectCmd())

	cobra.OnInitialize(initViper)
	return cmd
}

func defaultWorkers() int {
	n := runtime.NumCPU() - 2
	if n < 1 {
		return 1
	}
	return n
}

// initViper loads config file + env, then merges a selected preset on top.
func initViper() {
	viper.SetEnvPrefix("HUSH")
	viper.SetEnvKeyReplacer(strings.NewReplacer("-", "_"))
	viper.AutomaticEnv()

	if configFile != "" {
		viper.SetConfigFile(configFile)
	} else {
		viper.SetConfigName(".hush")
		viper.SetConfigType("yaml")
		viper.AddConfigPath(".")
		if home, err := os.UserHomeDir(); err == nil {
			viper.AddConfigPath(home)
		}
	}
	// Config is optional; swallow missing-file errors silently.
	if err := viper.ReadInConfig(); err != nil {
		if _, ok := err.(viper.ConfigFileNotFoundError); !ok {
			fmt.Fprintf(os.Stderr, "hush: config: %v\n", err)
		}
	}

	// If the user chose a preset, merge its fields on top of top-level config.
	if presetName != "" {
		sub := viper.Sub("presets." + presetName)
		if sub == nil {
			fmt.Fprintf(os.Stderr, "hush: preset %q not found; ignoring\n", presetName)
			return
		}
		for _, k := range sub.AllKeys() {
			viper.Set(k, sub.Get(k))
		}
	}
}

// Run executes the root command. Returns the process exit code.
func Run() int {
	cmd := NewRootCmd()
	if err := cmd.Execute(); err != nil {
		fmt.Fprintf(os.Stderr, "hush: %v\n", err)
		if exitErr, ok := err.(*ExitError); ok {
			return exitErr.Code
		}
		return 1
	}
	return 0
}

// ExitError lets handlers signal a specific exit code (e.g. 2 for --fail-on-finding).
type ExitError struct{ Code int }

func (e *ExitError) Error() string { return fmt.Sprintf("exit %d", e.Code) }

// -- small helpers used by scan.go to read viper+flag merged values ----

func bindFlag(cmd *cobra.Command, name string) {
	_ = viper.BindPFlag(name, cmd.Flags().Lookup(name))
}

// configPath returns a sensible label for messages.
func configPath() string {
	p := viper.ConfigFileUsed()
	if p == "" {
		return "(none)"
	}
	if abs, err := filepath.Abs(p); err == nil {
		return abs
	}
	return p
}
