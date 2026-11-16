package cli

import "github.com/rsdoiel/rptop/internal/exitcode"

// ErrUsage marks a bad command line; wrap it with %w to exit 2.
var ErrUsage = exitcode.ErrUsage

/** ExitCode is the one place an error becomes an exit code (DR-0002 9),
 * classifying by sentinel and never by message text.
 *
 * Parameters:
 *   err (error) — nil, or an error wrapping a collector sentinel or ErrUsage
 *
 * Returns:
 *   int — 0 for nil, the class code, or ExitInternal for an unclassified error
 *
 * Example:
 *   os.Exit(cli.ExitCode(fmt.Errorf("fan: %w", collector.ErrAbsent))) // 66
 */
func ExitCode(err error) int { return exitcode.Code(err) }

/** ClassName returns the DR-0003 class name of an exit code, for the "class"
 * field of a JSON error object.
 *
 * Parameters:
 *   code (int) — an exit code
 *
 * Returns:
 *   string — "no_input" for 66 and so on; "internal" for an unlisted code
 *
 * Example:
 *   cli.ClassName(66) // "no_input"
 */
func ClassName(code int) string { return exitcode.Class(code) }
