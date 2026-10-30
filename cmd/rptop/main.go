// Command rptop is a Raspberry Pi vitals dashboard with a scriptable,
// verb-oriented command line.
package main

import (
	"os"

	"github.com/rsdoiel/rptop/internal/cli"
)

func main() {
	os.Exit(cli.Run(os.Args[1:], os.Stdout, os.Stderr))
}
