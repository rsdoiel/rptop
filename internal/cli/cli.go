// Package cli is the rptop command line: option parsing, verb dispatch and
// exit codes. It is kept apart from cmd/rptop so that all of it can be tested
// with the standard testing package.
package cli

import (
	"flag"
	"fmt"
	"io"

	"github.com/rsdoiel/rptop"
)

// Exit codes, per workspace DR-0003. Only the classes rptop can produce are
// listed; the rest of the table is unused.
const (
	ExitOK           = 0
	ExitNegative     = 1
	ExitUsage        = 2
	ExitData         = 65
	ExitNoInput      = 66
	ExitUnavailable  = 69
	ExitInternal     = 70
	ExitCantCreate   = 73
	ExitIO           = 74
	ExitTempFail     = 75
	ExitNoPermission = 77
	ExitConfig       = 78
)

const appName = "rptop"

// standardOptions are the options every program of the workspace accepts.
type standardOptions struct {
	help, license, version bool
}

// register adds -h/-help, and for the top level -l/-license and -v/-version,
// to fs, each long and short name sharing one variable.
func (o *standardOptions) register(fs *flag.FlagSet, topLevel bool) {
	fs.BoolVar(&o.help, "h", false, "display help")
	fs.BoolVar(&o.help, "help", false, "display help")
	if topLevel {
		fs.BoolVar(&o.license, "l", false, "display license")
		fs.BoolVar(&o.license, "license", false, "display license")
		fs.BoolVar(&o.version, "v", false, "display version")
		fs.BoolVar(&o.version, "version", false, "display version")
	}
}

// helpPage writes text to out with the version markup filled in.
func helpPage(out io.Writer, text string) {
	io.WriteString(out, rptop.FmtHelp(text, appName, rptop.Version, rptop.ReleaseDate, rptop.ReleaseHash))
}

/** Run is the testable body of main.
 *
 * Parameters:
 *   args   ([]string)  — the command line without the program name
 *   stdout (io.Writer) — normal output
 *   stderr (io.Writer) — errors and warnings
 *
 * Returns:
 *   int — the process exit code
 *
 * Example:
 *   os.Exit(cli.Run(os.Args[1:], os.Stdout, os.Stderr))
 */
func Run(args []string, stdout, stderr io.Writer) int {
	return RunEnv(DefaultEnv(), args, stdout, stderr)
}

/** RunEnv is Run with the machine injected, so tests read fixtures instead of
 * the real /sys, /proc and Hailo device.
 *
 * Parameters:
 *   env    (Env)       — where sensors are read from
 *   args   ([]string)  — the command line without the program name
 *   stdout (io.Writer) — normal output
 *   stderr (io.Writer) — errors and warnings
 *
 * Returns:
 *   int — the process exit code
 *
 * Example:
 *   code := cli.RunEnv(cli.DefaultEnv(), []string{"temp", "soc"}, os.Stdout, os.Stderr)
 */
func RunEnv(env Env, args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet(appName, flag.ContinueOnError)
	fs.SetOutput(stderr)
	fs.Usage = func() {}
	var std standardOptions
	std.register(fs, true)

	if err := fs.Parse(args); err != nil {
		fmt.Fprintf(stderr, "Run '%s -help' for usage.\n", appName)
		return ExitUsage
	}
	switch {
	case std.help:
		helpPage(stdout, HelpText)
		return ExitOK
	case std.license:
		fmt.Fprint(stdout, rptop.LicenseText)
		return ExitOK
	case std.version:
		fmt.Fprintf(stdout, "%s %s %s\n", appName, rptop.Version, rptop.ReleaseHash)
		return ExitOK
	case fs.NArg() == 0:
		helpPage(stdout, HelpText)
		return ExitOK
	}

	verb, rest := fs.Arg(0), fs.Args()[1:]
	if verb == "help" {
		return runHelp(rest, stdout, stderr)
	}
	if _, ok := verbHelp[verb]; ok {
		return runVerb(env, verb, rest, stdout, stderr)
	}
	fmt.Fprintf(stderr, "%s: unknown verb %q. Run '%s -help' for usage.\n", appName, verb, appName)
	return ExitUsage
}

// runHelp implements `rptop help [VERB]`.
func runHelp(args []string, stdout, stderr io.Writer) int {
	switch len(args) {
	case 0:
		helpPage(stdout, HelpText)
		return ExitOK
	case 1:
		if text, ok := verbHelp[args[0]]; ok {
			helpPage(stdout, text)
			return ExitOK
		}
		fmt.Fprintf(stderr, "%s help: no help for %q\n", appName, args[0])
		return ExitUsage
	}
	fmt.Fprintf(stderr, "%s help: takes at most one verb\n", appName)
	return ExitUsage
}
