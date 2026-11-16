// Package exitcode maps errors to the workspace exit codes (DR-0003) and names
// the classes. It sits below cli so the sampler and the recorder can name a
// failure class without importing the command line.
package exitcode

import (
	"errors"

	"github.com/rsdoiel/rptop/internal/collector"
)

// Exit codes, per workspace DR-0003.
const (
	OK           = 0
	Negative     = 1
	Usage        = 2
	Data         = 65
	NoInput      = 66
	Unavailable  = 69
	Internal     = 70
	CantCreate   = 73
	IO           = 74
	TempFail     = 75
	NoPermission = 77
	Config       = 78
)

// ErrUsage marks a bad command line; wrap it with %w to exit 2.
var ErrUsage = errors.New("usage")

// ErrCantCreate marks an output that cannot be created: the file exists or its
// directory cannot be made. Wrap it with %w to exit 73.
var ErrCantCreate = errors.New("cannot create")

// ErrIO marks a read or write that failed part way, or a disk that is too
// full to go on. Wrap it with %w to exit 74.
var ErrIO = errors.New("i/o failure")

/** Code is the one place an error becomes an exit code (DR-0002 9). It
 * classifies by sentinel with errors.Is, never by message text.
 *
 * Parameters:
 *   err (error) — nil, or an error wrapping a collector sentinel or ErrUsage
 *
 * Returns:
 *   int — 0 for nil, the class code, or Internal for an unclassified error
 *
 * Example:
 *   exitcode.Code(fmt.Errorf("fan: %w", collector.ErrAbsent)) // 66
 */
func Code(err error) int {
	switch {
	case err == nil:
		return OK
	case errors.Is(err, ErrUsage):
		return Usage
	case errors.Is(err, ErrCantCreate):
		return CantCreate
	case errors.Is(err, ErrIO):
		return IO
	case errors.Is(err, collector.ErrAbsent):
		return NoInput
	case errors.Is(err, collector.ErrUnreachable):
		return Unavailable
	case errors.Is(err, collector.ErrUnparseable):
		return Data
	case errors.Is(err, collector.ErrRefused):
		return NoPermission
	}
	return Internal
}

/** Class returns the DR-0003 class name of an exit code.
 *
 * Parameters:
 *   code (int) — an exit code
 *
 * Returns:
 *   string — "no_input" for 66 and so on; "internal" for an unlisted code
 *
 * Example:
 *   exitcode.Class(66) // "no_input"
 */
func Class(code int) string {
	switch code {
	case OK:
		return "ok"
	case Negative:
		return "negative"
	case Usage:
		return "usage"
	case Data:
		return "data"
	case NoInput:
		return "no_input"
	case Unavailable:
		return "unavailable"
	case CantCreate:
		return "cant_create"
	case IO:
		return "io"
	case TempFail:
		return "temp_fail"
	case NoPermission:
		return "no_permission"
	case Config:
		return "config"
	}
	return "internal"
}
