package cli

import (
	"context"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/rsdoiel/rptop/internal/element"
	"github.com/rsdoiel/rptop/internal/recorder"
)

/** Env is the machine the command line reads, plus how it waits between the
 * two samples of a rate. Tests inject fixtures and a fake Sleep.
 *
 * Fields:
 *   Env   (element.Env)       — sensor sources and the Hailo opener (embedded)
 *   Sleep (func(time.Duration)) — nil means time.Sleep
 *   Context (func() (context.Context, context.CancelFunc)) — what ends `record`; nil means never (only -duration ends it)
 *   Ticks (<-chan time.Time) — drives the sampler in tests; nil means a real ticker
 *   Now (func() time.Time)   — the clock for the sampler; nil means time.Now
 *   Recorder (recorder.Options) — tuning for the recorder; the zero value is the default
 *
 * Example:
 *   code := cli.RunEnv(cli.DefaultEnv(), args, os.Stdout, os.Stderr)
 */
type Env struct {
	element.Env
	Sleep    func(time.Duration)
	Context  func() (context.Context, context.CancelFunc)
	Ticks    <-chan time.Time
	Now      func() time.Time
	Recorder recorder.Options
}

/** DefaultEnv returns the Env for the running Pi.
 *
 * Returns:
 *   Env — real sources, the real Hailo opener, time.Sleep and a Context cancelled by SIGINT or SIGTERM
 *
 * Example:
 *   env := cli.DefaultEnv()
 */
func DefaultEnv() Env {
	return Env{
		Env:   element.DefaultEnv(),
		Sleep: time.Sleep,
		// Created only by `record`, so Ctrl-C still kills every other verb.
		Context: func() (context.Context, context.CancelFunc) {
			return signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
		},
	}
}

func (e Env) sleep(d time.Duration) {
	if e.Sleep != nil {
		e.Sleep(d)
		return
	}
	time.Sleep(d)
}
