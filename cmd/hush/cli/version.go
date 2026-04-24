package cli

import (
	"fmt"

	"github.com/spf13/cobra"
)

// NewVersionCmd prints version + build info.
func NewVersionCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "version",
		Short: "Print hush version.",
		Run: func(c *cobra.Command, args []string) {
			fmt.Printf("hush %s (extractor + BitNet int8 embedded)\n", Version)
			fmt.Printf("config: %s\n", configPath())
		},
	}
}
