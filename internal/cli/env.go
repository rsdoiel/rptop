package cli

import (
	"context"
	"os"
	"os/signal"
	"syscall"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/term"

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
 *   IsTerminal (func() bool)    — whether standard input and output are a terminal; nil means no
 *   NewProgram (func(tea.Model) Program) — builds the dashboard's program; nil means a real Bubble Tea program
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

	IsTerminal func() bool
	NewProgram func(tea.Model) Program
}

/** Program is the part of a Bubble Tea program the command line uses; tests
 * substitute one that runs the model without a terminal.
 *
 * Example:
 *   var p cli.Program = realProgram
 */
type Program interface {
	Run() error
	Send(tea.Msg)
}

type teaProgram struct{ p *tea.Program }

func (t teaProgram) Run() error     { _, err := t.p.Run(); return err }
func (t teaProgram) Send(m tea.Msg) { t.p.Send(m) }

// stdioIsTerminal reports whether standard input and output are both terminals.
// A character device is not enough: /dev/null is one.
func stdioIsTerminal() bool {
	return term.IsTerminal(os.Stdin.Fd()) && term.IsTerminal(os.Stdout.Fd())
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
		Env:        element.DefaultEnv(),
		Sleep:      time.Sleep,
		IsTerminal: stdioIsTerminal,
		NewProgram: func(m tea.Model) Program { return teaProgram{tea.NewProgram(m)} },
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
