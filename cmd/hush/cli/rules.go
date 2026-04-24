package cli

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/valllabh/hush/pkg/extractor"
)

// NewRulesCmd lists or dumps the built-in regex rules.
//
//	hush rules                     # human-readable table
//	hush rules --json              # dump defaults as JSON (use to bootstrap --rules-file)
//	hush rules --json > rules.json
func NewRulesCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "rules",
		Short: "List or dump the built-in regex rules.",
		Long: `List or dump the built-in regex rules.

The defaults ship embedded in the binary. Override or extend them with
--rules-file <path>; JSON schema:

    {"rules": [{"name":"internal_tok","pattern":"\\bmytok_..."}],
     "disabled": ["aws_access_key_id"]}

To start from the built-in set, dump and edit:

    hush rules --json > rules.json
    # edit rules.json: add new entries to "rules", list disables in "disabled"
    hush --rules-file rules.json <paths>`,
		Run: func(c *cobra.Command, args []string) {
			asJSON, _ := c.Flags().GetBool("json")
			if asJSON {
				fmt.Print(string(extractor.DefaultRulesJSON()))
				return
			}
			printRulesTable(os.Stdout)
		},
	}
	cmd.Flags().Bool("json", false, "Dump the default rules as JSON (copy-paste ready for --rules-file)")
	return cmd
}

func printRulesTable(w *os.File) {
	fmt.Fprintf(w, "%-28s  %s\n", "NAME", "PATTERN")
	fmt.Fprintln(w, "------------------------------  --------------------------------")
	for _, r := range extractor.Rules {
		fmt.Fprintf(w, "%-28s  %s\n", r.Name, r.Regex.String())
	}
	fmt.Fprintln(w)
	fmt.Fprintln(w, "Plus: high-entropy Shannon scan (threshold via --entropy-threshold)")
}

// unused: kept for future `rules --active` which would print the merged set
// after --rules-file is applied.
var _ = json.Marshal
