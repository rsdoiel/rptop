package cli

import (
	"bytes"
	"strings"
	"testing"

	"github.com/rsdoiel/rptop"
)

func TestVersionAllSpellings(t *testing.T) {
	for _, opt := range []string{"-v", "-version", "--version"} {
		var out, errOut bytes.Buffer
		if code := Run([]string{opt}, &out, &errOut); code != ExitOK {
			t.Fatalf("%s: exit code = %d, want %d", opt, code, ExitOK)
		}
		want := "rptop " + rptop.Version + " " + rptop.ReleaseHash + "\n"
		if out.String() != want {
			t.Errorf("%s: stdout = %q, want %q", opt, out.String(), want)
		}
	}
}

func TestLicenseAllSpellings(t *testing.T) {
	for _, opt := range []string{"-l", "-license", "--license"} {
		var out, errOut bytes.Buffer
		if code := Run([]string{opt}, &out, &errOut); code != ExitOK {
			t.Fatalf("%s: exit code = %d, want %d", opt, code, ExitOK)
		}
		if !strings.Contains(out.String(), "GNU Affero General Public License") {
			t.Errorf("%s: stdout is not the license: %q", opt, out.String())
		}
	}
}

func TestHelpAllSpellingsFillsTheMarkup(t *testing.T) {
	for _, args := range [][]string{{"-h"}, {"-help"}, {"--help"}, {"help"}, {}} {
		var out, errOut bytes.Buffer
		if code := Run(args, &out, &errOut); code != ExitOK {
			t.Fatalf("%v: exit code = %d, want %d", args, code, ExitOK)
		}
		if strings.Contains(out.String(), "{app_name}") || strings.Contains(out.String(), "{version}") {
			t.Errorf("%v: help still has unexpanded markup", args)
		}
		if !strings.Contains(out.String(), "rptop(1)") {
			t.Errorf("%v: help is not the rptop man page", args)
		}
	}
}

// DR-0003: a bad command line is 2 and nothing is attempted.
func TestUsageErrorsExitTwo(t *testing.T) {
	for _, args := range [][]string{
		{"-no-such-option"},
		{"--no-such-option"},
		{"nosuchverb"},
		{"help", "nosuchverb"},
		{"help", "a", "b"},
	} {
		var out, errOut bytes.Buffer
		if code := Run(args, &out, &errOut); code != ExitUsage {
			t.Errorf("%v: exit code = %d, want %d", args, code, ExitUsage)
		}
		if out.Len() != 0 {
			t.Errorf("%v: wrote to stdout on a usage error: %q", args, out.String())
		}
		if errOut.Len() == 0 {
			t.Errorf("%v: said nothing on stderr", args)
		}
	}
}

// DR-0003 lists the codes; the constants must keep the BSD sysexits values.
func TestExitCodeValues(t *testing.T) {
	want := map[string]struct{ got, want int }{
		"ok": {ExitOK, 0}, "negative": {ExitNegative, 1}, "usage": {ExitUsage, 2},
		"data": {ExitData, 65}, "no_input": {ExitNoInput, 66}, "unavailable": {ExitUnavailable, 69},
		"internal": {ExitInternal, 70}, "cant_create": {ExitCantCreate, 73}, "io": {ExitIO, 74},
		"temp_fail": {ExitTempFail, 75}, "no_permission": {ExitNoPermission, 77}, "config": {ExitConfig, 78},
	}
	for name, c := range want {
		if c.got != c.want {
			t.Errorf("Exit%s = %d, want %d", name, c.got, c.want)
		}
	}
}
