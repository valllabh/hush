// hush: portable secrets and PII scrubber.
package main

import (
	"os"

	"github.com/valllabh/hush/cmd/hush/cli"
)

func main() {
	os.Exit(cli.Run())
}
