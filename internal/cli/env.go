package cli

import (
	"time"

	"github.com/rsdoiel/rptop/internal/element"
)

/** Env is the machine the command line reads, plus how it waits between the
 * two samples of a rate. Tests inject fixtures and a fake Sleep.
 *
 * Fields:
 *   Env   (element.Env)       — sensor sources and the Hailo opener (embedded)
 *   Sleep (func(time.Duration)) — nil means time.Sleep
 *
 * Example:
 *   code := cli.RunEnv(cli.DefaultEnv(), args, os.Stdout, os.Stderr)
 */
type Env struct {
	element.Env
	Sleep func(time.Duration)
}

/** DefaultEnv returns the Env for the running Pi.
 *
 * Returns:
 *   Env — real sources, the real Hailo opener and time.Sleep
 *
 * Example:
 *   env := cli.DefaultEnv()
 */
func DefaultEnv() Env { return Env{Env: element.DefaultEnv(), Sleep: time.Sleep} }

func (e Env) sleep(d time.Duration) {
	if e.Sleep != nil {
		e.Sleep(d)
		return
	}
	time.Sleep(d)
}
