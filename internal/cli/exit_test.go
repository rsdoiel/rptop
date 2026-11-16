package cli

import (
	"errors"
	"fmt"
	"testing"

	"github.com/rsdoiel/rptop/internal/collector"
	"github.com/rsdoiel/rptop/internal/exitcode"
)

func TestExitCode(t *testing.T) {
	cases := []struct {
		err  error
		want int
	}{
		{nil, ExitOK},
		{fmt.Errorf("x: %w", collector.ErrAbsent), ExitNoInput},
		{fmt.Errorf("x: %w", collector.ErrUnreachable), ExitUnavailable},
		{fmt.Errorf("x: %w", collector.ErrUnparseable), ExitData},
		{fmt.Errorf("x: %w", collector.ErrRefused), ExitNoPermission},
		{fmt.Errorf("x: %w", exitcode.ErrCantCreate), ExitCantCreate},
		{fmt.Errorf("x: %w", exitcode.ErrIO), ExitIO},
		{ErrUsage, ExitUsage},
		{fmt.Errorf("bad: %w", ErrUsage), ExitUsage},
		{errors.New("nothing classified this"), ExitInternal},
	}
	for _, c := range cases {
		if got := ExitCode(c.err); got != c.want {
			t.Errorf("ExitCode(%v) = %d, want %d", c.err, got, c.want)
		}
	}
}

func TestClassName(t *testing.T) {
	if got := ClassName(ExitNoInput); got != "no_input" {
		t.Errorf("ClassName(66) = %q", got)
	}
	if got := ClassName(ExitUnavailable); got != "unavailable" {
		t.Errorf("ClassName(69) = %q", got)
	}
}
