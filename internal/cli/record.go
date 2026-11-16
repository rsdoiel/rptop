package cli

import (
	"context"
	"errors"
	"fmt"
	"io"
	"sync/atomic"
	"time"

	"github.com/rsdoiel/rptop"
	"github.com/rsdoiel/rptop/internal/recorder"
	"github.com/rsdoiel/rptop/internal/sample"
	"github.com/rsdoiel/rptop/internal/sampler"
)

// runRecord is `rptop record`: the sampler feeding a recorder in the
// foreground until Ctrl-C, SIGTERM or -duration (DR-0002 7, DR-0003).
func runRecord(env Env, args []string, stdout, stderr io.Writer) int {
	const verb = "record"
	fs, std := newVerbFlags(verb)
	var label, out string
	interval := time.Second
	var duration time.Duration
	fs.StringVar(&label, "label", "", "a name for the run")
	fs.StringVar(&out, "out", "", "output file or directory")
	fs.DurationVar(&interval, "interval", time.Second, "time between samples")
	fs.DurationVar(&interval, "i", time.Second, "time between samples")
	fs.DurationVar(&duration, "duration", 0, "stop after this long")
	pos, err := parseInterleaved(fs, args)
	if err != nil {
		return usageErr(stderr, verb, err)
	}
	if std.help {
		helpPage(stdout, verbHelp[verb])
		return ExitOK
	}
	if len(pos) > 0 {
		return usageErr(stderr, verb, fmt.Errorf("surplus argument %q", pos[0]))
	}
	if duration < 0 {
		return usageErr(stderr, verb, errors.New("-duration must not be negative"))
	}
	smp, err := sampler.New(sampler.Config{
		Env: env.Env, Label: label, Version: rptop.Version + " " + rptop.ReleaseHash,
		Interval: interval, Ticks: env.Ticks, Now: env.Now,
	})
	if err != nil {
		if ExitCode(err) == ExitUsage {
			return usageErr(stderr, verb, err)
		}
		return reportError(stdout, stderr, false, verb, verb, err)
	}
	defer smp.Close()

	opts := env.Recorder
	if opts.Now == nil {
		opts.Now = env.Now
	}
	ctl := recorder.NewController(smp.Meta, opts)
	st, err := ctl.Start(label, out)
	if err != nil {
		return reportError(stdout, stderr, false, verb, verb, err)
	}
	fmt.Fprintf(stderr, "%s: recording to %s (Ctrl-C to stop)\n", appName, st.Path)

	parent, stopParent := context.Background(), context.CancelFunc(func() {})
	if env.Context != nil {
		parent, stopParent = env.Context()
	}
	defer stopParent()
	ctx, cancel := context.WithCancel(parent)
	defer cancel()
	var durationUp atomic.Bool
	if duration > 0 {
		timer := time.AfterFunc(duration, func() { durationUp.Store(true); cancel() })
		defer timer.Stop()
	}

	var feedErr error
	smp.Run(ctx, func(x sample.Sample) {
		if feedErr == nil {
			if err := ctl.Feed(x); err != nil {
				feedErr = err
				cancel()
			}
		}
	})
	if feedErr != nil { // the controller has already written meta_end
		reportError(stdout, stderr, false, verb, verb, feedErr)
		return ExitCode(feedErr)
	}
	reason := sample.ReasonInterrupt
	if durationUp.Load() {
		reason = sample.ReasonDuration
	}
	end, err := ctl.Stop(reason)
	if err != nil {
		return reportError(stdout, stderr, false, verb, verb, err)
	}
	fmt.Fprintf(stderr, "%s: record: %d samples, %s\n", appName, end.Samples, end.Reason)
	fmt.Fprintln(stdout, end.Path)
	return ExitOK
}
