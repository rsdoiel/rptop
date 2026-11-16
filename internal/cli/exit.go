package cli

import (
	"errors"

	"github.com/rsdoiel/rptop/internal/collector"
)

// ErrUsage marks a bad command line; wrap it with %w to exit 2.
var ErrUsage = errors.New("usage")

/** ExitCode is the one place an error becomes an exit code (DR-0002 9).
 * It classifies by sentinel with errors.Is, never by message text.
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
func ExitCode(err error) int {
	switch {
	case err == nil:
		return ExitOK
	case errors.Is(err, ErrUsage):
		return ExitUsage
	case errors.Is(err, collector.ErrAbsent):
		return ExitNoInput
	case errors.Is(err, collector.ErrUnreachable):
		return ExitUnavailable
	case errors.Is(err, collector.ErrUnparseable):
		return ExitData
	case errors.Is(err, collector.ErrRefused):
		return ExitNoPermission
	}
	return ExitInternal
}

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
func ClassName(code int) string {
	switch code {
	case ExitOK:
		return "ok"
	case ExitNegative:
		return "negative"
	case ExitUsage:
		return "usage"
	case ExitData:
		return "data"
	case ExitNoInput:
		return "no_input"
	case ExitUnavailable:
		return "unavailable"
	case ExitCantCreate:
		return "cant_create"
	case ExitIO:
		return "io"
	case ExitTempFail:
		return "temp_fail"
	case ExitNoPermission:
		return "no_permission"
	case ExitConfig:
		return "config"
	}
	return "internal"
}
