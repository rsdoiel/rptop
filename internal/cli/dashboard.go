package cli

import (
	"context"
	"flag"
	"fmt"
	"io"
	"os"
	"sync"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/rsdoiel/rptop"
	"github.com/rsdoiel/rptop/internal/collector"
	"github.com/rsdoiel/rptop/internal/recorder"
	"github.com/rsdoiel/rptop/internal/sample"
	"github.com/rsdoiel/rptop/internal/sampler"
	"github.com/rsdoiel/rptop/internal/ui"
)

// Defaults of the bare command (DR-0004 2).
const (
	defaultWarnTemp = 60.0
	defaultCritTemp = 80.0
)

// dashboardOptions are the options of the bare `rptop`; they are the whole of
// its configuration (DR-0004 1).
type dashboardOptions struct {
	interval time.Duration
	warn     float64
	crit     float64
	out      string
}

func (d *dashboardOptions) register(fs *flag.FlagSet) {
	fs.DurationVar(&d.interval, "interval", time.Second, "time between samples")
	fs.DurationVar(&d.interval, "i", time.Second, "time between samples")
	fs.Float64Var(&d.warn, "warn-temp", defaultWarnTemp, "temperatures at or above this are yellow")
	fs.Float64Var(&d.crit, "crit-temp", defaultCritTemp, "temperatures at or above this are red")
	fs.StringVar(&d.out, "out", "", "where recordings go")
}

// given returns the name of a dashboard option set on the command line, or "".
func (d *dashboardOptions) given(fs *flag.FlagSet) string {
	name := ""
	fs.Visit(func(f *flag.Flag) {
		switch f.Name {
		case "interval", "i", "warn-temp", "crit-temp", "out":
			if name == "" {
				name = f.Name
			}
		}
	})
	return name
}

/** runDashboard is the bare `rptop`: it checks the options and the terminal,
 * then runs the sampler, the recorder and the Bubble Tea program together
 * until the person quits or the context ends.
 *
 * Parameters:
 *   env    (Env)               — the machine, the terminal test and the program factory
 *   d      (*dashboardOptions) — the parsed options
 *   stdout, stderr (io.Writer) — help goes to stderr when there is no terminal
 *
 * Returns:
 *   int — 0 after a normal quit, 2 for a bad option or no terminal, 70 for a failure of the program itself
 *
 * Example:
 *   code := runDashboard(env, &opts, os.Stdout, os.Stderr)
 */
func runDashboard(env Env, d *dashboardOptions, stdout, stderr io.Writer) int {
	if d.interval < sampler.MinInterval {
		return usageErr(stderr, "dashboard", fmt.Errorf("-interval must be at least %v", sampler.MinInterval))
	}
	if d.crit <= d.warn {
		return usageErr(stderr, "dashboard", fmt.Errorf("-crit-temp (%g) must be above -warn-temp (%g)", d.crit, d.warn))
	}
	if env.IsTerminal == nil || !env.IsTerminal() {
		helpPage(stderr, HelpText)
		return ExitUsage
	}

	smp, err := sampler.New(sampler.Config{
		Env: env.Env, Version: rptop.Version + " " + rptop.ReleaseHash,
		Interval: d.interval, Ticks: env.Ticks, Now: env.Now,
	})
	if err != nil {
		return reportError(stdout, stderr, false, "dashboard", "dashboard", err)
	}
	defer smp.Close()
	opts := env.Recorder
	if opts.Now == nil {
		opts.Now = env.Now
	}
	ctl := recorder.NewController(smp.Meta, opts)
	host, _ := os.Hostname()
	model := ui.New(ui.Options{
		Host: host, Interval: d.interval, WarnTemp: d.warn, CritTemp: d.crit, Out: d.out,
		Uptime: func() time.Duration { up, _ := collector.Uptime(env.Sources); return up },
	}, ctl)
	newProgram := env.NewProgram
	if newProgram == nil {
		newProgram = func(m tea.Model) Program { return teaProgram{tea.NewProgram(m)} }
	}
	prog := newProgram(model)

	parent, stopParent := context.Background(), context.CancelFunc(func() {})
	if env.Context != nil {
		parent, stopParent = env.Context()
	}
	defer stopParent()
	ctx, cancel := context.WithCancel(parent)
	defer cancel()

	var wg sync.WaitGroup
	wg.Add(2)
	go func() { // one sampler feeds the screen and the recording alike
		defer wg.Done()
		smp.Run(ctx, func(x sample.Sample) {
			if err := ctl.Feed(x); err != nil {
				prog.Send(ui.RecEndedMsg{Status: ctl.Status(), Err: err})
			}
			prog.Send(ui.SampleMsg{S: x})
		})
	}()
	go func() { // SIGTERM or the like: leave the way q does
		defer wg.Done()
		<-ctx.Done()
		prog.Send(tea.QuitMsg{})
	}()

	runErr := prog.Run()
	cancel()
	wg.Wait()
	ctl.Stop(sample.ReasonStopped) // a no-op unless a recording is still open
	if runErr != nil {
		return reportError(stdout, stderr, false, "dashboard", "dashboard", runErr)
	}
	return ExitOK
}
