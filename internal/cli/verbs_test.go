package cli

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/rsdoiel/rptop/internal/collector"
	"github.com/rsdoiel/rptop/internal/testenv"
)

const fixtureDir = "../../testdata/pi5-trixie"

type fixture struct {
	*testenv.Fixture
	t     *testing.T
	slept []time.Duration
}

// newFixture copies the captured Pi 5 files to a temp directory so a fake
// Sleep can advance the counters, and returns an Env reading them.
func newFixture(t *testing.T) (*fixture, Env) {
	t.Helper()
	f := &fixture{Fixture: testenv.New(t, fixtureDir), t: t}
	env := Env{Env: f.Env}
	env.Sleep = func(d time.Duration) {
		f.slept = append(f.slept, d)
		f.Advance()
	}
	return f, env
}

func run(env Env, args ...string) (code int, stdout, stderr string) {
	var out, errOut bytes.Buffer
	code = RunEnv(env, args, &out, &errOut)
	return code, out.String(), errOut.String()
}

func TestSingleReadingForms(t *testing.T) {
	_, env := newFixture(t)
	cases := []struct {
		args []string
		want string
	}{
		{[]string{"temp", "soc"}, "55.1 °C\n"},
		{[]string{"temp", "soc", "-n"}, "55.1\n"},
		{[]string{"temp", "soc", "-number"}, "55.1\n"},
		{[]string{"temp", "soc", "-j"}, `{"name":"temp.soc","value":55.1,"unit":"°C"}` + "\n"},
		{[]string{"temp", "-json", "pmic"}, `{"name":"temp.pmic","value":54.3,"unit":"°C"}` + "\n"},
		{[]string{"temp", "rp1"}, "55.5 °C\n"},
		{[]string{"fan", "speed"}, "3725 rpm\n"},
		{[]string{"fan"}, "speed 3725 rpm\n"},
		{[]string{"volts", "core"}, "0.7838 V\n"},
		{[]string{"cpu", "freq"}, "1800 MHz\n"},
		{[]string{"cpu", "load"}, "0.08\n"},
		{[]string{"mem", "total"}, "17005985792 bytes\n"},
	}
	for _, c := range cases {
		code, out, errOut := run(env, c.args...)
		if code != ExitOK || out != c.want {
			t.Errorf("%v: code %d out %q (stderr %q), want 0 %q", c.args, code, out, errOut, c.want)
		}
	}
}

func TestThresholds(t *testing.T) {
	_, env := newFixture(t)
	cases := []struct {
		args []string
		code int
	}{
		{[]string{"temp", "soc", "-above", "70"}, ExitNegative},
		{[]string{"temp", "soc", "-above", "50"}, ExitOK},
		{[]string{"temp", "soc", "-below", "70"}, ExitOK},
		{[]string{"temp", "soc", "-below", "50"}, ExitNegative},
		{[]string{"temp", "soc", "-above", "50", "-below", "60"}, ExitOK},
		{[]string{"temp", "soc", "-above", "50", "-below", "55"}, ExitNegative},
		{[]string{"-above", "50", "temp", "soc"}, ExitUsage}, // options belong to the verb
	}
	for _, c := range cases {
		code, out, _ := run(env, c.args...)
		if code != c.code {
			t.Errorf("%v: code %d, want %d", c.args, code, c.code)
		}
		if c.code != ExitUsage && out != "55.1 °C\n" {
			t.Errorf("%v: reading not printed with the check: %q", c.args, out)
		}
	}
}

func TestGroupReportsEveryElementAndBulkRule(t *testing.T) {
	_, env := newFixture(t) // no Hailo: the stub reports it absent
	code, out, errOut := run(env, "temp")
	if code != ExitNoInput {
		t.Errorf("code %d, want %d", code, ExitNoInput)
	}
	if want := "soc 55.1 °C\npmic 54.3 °C\nrp1 55.5 °C\n"; out != want {
		t.Errorf("stdout = %q, want %q", out, want)
	}
	if !strings.Contains(errOut, "rptop: temp hailo:") {
		t.Errorf("stderr lacks the failed element: %q", errOut)
	}
	if !strings.Contains(errOut, "3 read, 1 failed") {
		t.Errorf("stderr lacks the counts: %q", errOut)
	}
}

func TestGroupOnlyFirstFailureClassWins(t *testing.T) {
	_, env := newFixture(t)
	env.Sources.ThermalZone = filepath.Join(t.TempDir(), "gone") // soc absent: 66, first
	env.Sources.Vcgencmd = func(...string) ([]byte, error) { return []byte("garbage"), nil }
	code, _, _ := run(env, "temp")
	if code != ExitNoInput {
		t.Errorf("code %d, want %d (soc comes first, absent)", code, ExitNoInput)
	}
}

func TestJSONErrorObjects(t *testing.T) {
	_, env := newFixture(t)
	code, out, errOut := run(env, "temp", "-j")
	if code != ExitNoInput {
		t.Fatalf("code %d", code)
	}
	if errOut == "" {
		t.Error("error also goes to stderr as plain text")
	}
	lines := strings.Split(strings.TrimSpace(out), "\n")
	if len(lines) != 4 {
		t.Fatalf("want 3 readings and 1 error object on stdout, got %q", out)
	}
	for _, l := range lines {
		if !json.Valid([]byte(l)) {
			t.Errorf("stdout line is not JSON: %q", l)
		}
	}
	var e struct {
		Error, Class, Name string
		Code               int
	}
	for _, l := range lines {
		if strings.Contains(l, `"error"`) {
			json.Unmarshal([]byte(l), &e)
		}
	}
	if e.Class != "no_input" || e.Code != 66 || e.Error == "" || e.Name != "temp.hailo" {
		t.Errorf("error object = %+v", e)
	}
}

// One test per failure class a reading can produce (DR-0002 9).
func TestFailureClasses(t *testing.T) {
	_, env := newFixture(t)
	absent := env
	absent.Sources.HwmonDir = t.TempDir()
	if code, _, _ := run(absent, "fan", "speed"); code != ExitNoInput {
		t.Errorf("absent: code %d, want 66", code)
	}
	bad := env
	bad.Sources.Vcgencmd = func(...string) ([]byte, error) { return []byte("garbage"), nil }
	if code, _, _ := run(bad, "volts", "core"); code != ExitData {
		t.Errorf("unparseable: code %d, want 65", code)
	}
	unreachable := env
	unreachable.Sources.CPUFreq = t.TempDir()
	if code, _, _ := run(unreachable, "cpu", "freq"); code != ExitUnavailable {
		t.Errorf("unreachable: code %d, want 69", code)
	}
	unreachable.OpenHailo = func() (collector.HailoReader, error) {
		return nil, fmt.Errorf("hailo open: %w", collector.ErrUnreachable)
	}
	if code, _, _ := run(unreachable, "temp", "hailo"); code != ExitUnavailable {
		t.Errorf("hailo unreachable: code %d, want 69", code)
	}
	if os.Geteuid() != 0 {
		refused := env
		p := filepath.Join(t.TempDir(), "t")
		os.WriteFile(p, []byte("55100"), 0o000)
		refused.Sources.ThermalZone = p
		if code, _, _ := run(refused, "temp", "soc"); code != ExitNoPermission {
			t.Errorf("refused: code %d, want 77", code)
		}
	}
}

type fakeReader struct{ closed *bool }

func (f fakeReader) Temps() (collector.HailoTemps, error) {
	return collector.HailoTemps{TS0: 54.3, TS1: 53.9}, nil
}
func (f fakeReader) Close() error { *f.closed = true; return nil }

func TestHailoElementOpensOnceAndCloses(t *testing.T) {
	_, env := newFixture(t)
	closed := false
	opens := 0
	env.OpenHailo = func() (collector.HailoReader, error) { opens++; return fakeReader{&closed}, nil }
	code, out, _ := run(env, "temp")
	if code != ExitOK || !strings.Contains(out, "hailo 54.3 °C\n") {
		t.Errorf("code %d out %q", code, out)
	}
	if opens != 1 || !closed {
		t.Errorf("opens %d closed %v, want opened once and released", opens, closed)
	}
	// A verb that does not use Hailo never opens it.
	opens = 0
	run(env, "temp", "soc")
	if opens != 0 {
		t.Errorf("temp soc opened the Hailo device")
	}
}

func TestCPURatesSampleAcrossTheInterval(t *testing.T) {
	f, env := newFixture(t)
	code, out, errOut := run(env, "cpu", "total", "-interval", "2s")
	if code != ExitOK || out != "50.0 %\n" {
		t.Errorf("total: code %d out %q err %q", code, out, errOut)
	}
	if len(f.slept) != 1 || f.slept[0] != 2*time.Second {
		t.Errorf("slept %v, want one 2s sleep", f.slept)
	}
	_, env = newFixture(t)
	if code, out, _ := run(env, "cpu", "core", "1"); code != ExitOK || out != "30.0 %\n" {
		t.Errorf("core 1: code %d out %q", code, out)
	}
	_, env = newFixture(t)
	if code, out, _ := run(env, "cpu", "core", "1", "-j"); code != ExitOK || out != `{"name":"cpu.core1","value":30.0,"unit":"%"}`+"\n" {
		t.Errorf("core 1 json: code %d out %q", code, out)
	}
	_, env = newFixture(t)
	if code, _, _ := run(env, "cpu", "core", "9"); code != ExitNoInput {
		t.Errorf("core 9: code %d, want 66", code)
	}
}

func TestCPUGroupListsEveryCore(t *testing.T) {
	_, env := newFixture(t)
	code, out, _ := run(env, "cpu")
	if code != ExitOK {
		t.Fatalf("code %d", code)
	}
	for _, label := range []string{"total ", "core0 ", "core1 ", "core2 ", "core3 ", "freq ", "load "} {
		if !strings.Contains(out, label) {
			t.Errorf("group output lacks %q:\n%s", label, out)
		}
	}
}

func TestDiskAndNetRates(t *testing.T) {
	_, env := newFixture(t)
	code, out, _ := run(env, "net", "rx", "-interval", "2s")
	if code != ExitOK || out != "500 B/s\n" {
		t.Errorf("net rx: code %d out %q", code, out)
	}
	_, env = newFixture(t)
	code, out, _ = run(env, "disk", "read", "-interval", "2s")
	if code != ExitOK || out != "524288 B/s\n" {
		t.Errorf("disk read: code %d out %q", code, out)
	}
	_, env = newFixture(t)
	if code, out, _ := run(env, "disk"); code != ExitOK || out != "read 1048576 B/s\nwrite 0 B/s\n" {
		t.Errorf("disk group: code %d out %q", code, out)
	}
}

func TestMem(t *testing.T) {
	_, env := newFixture(t)
	code, out, _ := run(env, "mem")
	want := "used 1079115776 bytes\ntotal 17005985792 bytes\nswap 0 bytes\n"
	if code != ExitOK || out != want {
		t.Errorf("code %d out %q, want %q", code, out, want)
	}
}

func TestThrottle(t *testing.T) {
	_, env := newFixture(t)
	code, out, _ := run(env, "throttle")
	want := "now: none\nsince boot: arm frequency capped, throttled, soft temperature limit\n"
	if code != ExitOK || out != want {
		t.Errorf("code %d out %q, want %q", code, out, want)
	}
	if code, _, _ := run(env, "throttle", "-since-boot"); code != ExitNegative {
		t.Errorf("-since-boot with history: code %d, want 1", code)
	}
	active := env
	active.Sources.Vcgencmd = func(...string) ([]byte, error) { return []byte("throttled=0x5\n"), nil }
	code, out, _ = run(active, "throttle")
	if code != ExitNegative || !strings.HasPrefix(out, "now: under-voltage, throttled\n") {
		t.Errorf("active: code %d out %q, want 1", code, out)
	}
	code, out, _ = run(env, "throttle", "-j")
	if code != ExitOK || out != `{"name":"throttle","raw":"0xe0000","now":[],"since_boot":["arm frequency capped","throttled","soft temperature limit"]}`+"\n" {
		t.Errorf("json: code %d out %q", code, out)
	}
	none := env
	none.Sources.Vcgencmd = nil
	if code, _, _ := run(none, "throttle"); code != ExitNoInput {
		t.Errorf("no vcgencmd: code %d, want 66", code)
	}
}

func TestList(t *testing.T) {
	_, env := newFixture(t)
	code, out, _ := run(env, "list")
	if code != ExitOK {
		t.Fatalf("code %d", code)
	}
	for _, want := range []string{"temp.soc", "temp.hailo", "cpu.core3", "disk.write", "net.tx", "fan.speed", "volts.core", "mem.swap"} {
		if !strings.Contains(out, want) {
			t.Errorf("list lacks %s:\n%s", want, out)
		}
	}
	if !strings.Contains(out, "no_input") {
		t.Errorf("list does not show the absent Hailo element:\n%s", out)
	}
	_, out, _ = run(env, "list", "-j")
	for _, l := range strings.Split(strings.TrimSpace(out), "\n") {
		var row struct {
			Name      string
			Available bool
		}
		if err := json.Unmarshal([]byte(l), &row); err != nil || row.Name == "" {
			t.Errorf("bad list row %q: %v", l, err)
		}
	}
}

func TestVerbUsageErrors(t *testing.T) {
	_, env := newFixture(t)
	for _, args := range [][]string{
		{"temp", "nosuch"},
		{"temp", "soc", "pmic"},
		{"temp", "soc", "-above", "hot"},
		{"temp", "-no-such-flag"},
		{"fan", "speed", "extra"},
		{"cpu", "core"},
		{"cpu", "core", "x"},
		{"cpu", "core", "-1"},
		{"cpu", "core", "1", "2"},
		{"cpu", "total", "-interval", "0s"},
		{"cpu", "total", "-interval", "soon"},
		{"temp", "-n", "-j"},
		{"list", "temp"},
		{"throttle", "now"},
		{"throttle", "-n"},
		{"fan", "-interval", "1s"}, // not a rate verb
	} {
		code, out, errOut := run(env, args...)
		if code != ExitUsage {
			t.Errorf("%v: code %d, want 2", args, code)
		}
		if out != "" {
			t.Errorf("%v: stdout on a usage error: %q", args, out)
		}
		if !strings.HasPrefix(errOut, "rptop: ") {
			t.Errorf("%v: stderr = %q", args, errOut)
		}
	}
}

func TestVerbHelp(t *testing.T) {
	_, env := newFixture(t)
	for verb := range verbHelp {
		for _, args := range [][]string{{verb, "-h"}, {verb, "-help"}, {"help", verb}} {
			code, out, _ := run(env, args...)
			if code != ExitOK || !strings.Contains(out, "rptop-"+verb+"(1)") || strings.Contains(out, "{app_name}") {
				t.Errorf("%v: code %d, page %q", args, code, out)
			}
			if !strings.Contains(out, "# EXIT STATUS") {
				t.Errorf("%v: help has no EXIT STATUS", args)
			}
		}
	}
	if len(verbHelp) != 10 {
		t.Errorf("verbHelp has %d pages, want 10", len(verbHelp))
	}
}

func TestErrorsClassifiedBySentinelNotText(t *testing.T) {
	err := fmt.Errorf("the words not present on this machine appear here: %w", errors.New("other"))
	if ExitCode(err) != ExitInternal {
		t.Error("an error is classified by its text")
	}
}

// Each help page is the source of a man page. pandoc warns when it misreads
// something (a pair of $ signs in an example became TeX math), and a page that
// builds with a warning is a page that renders wrongly.
func TestHelpPagesConvertWithoutPandocWarnings(t *testing.T) {
	pandoc, err := exec.LookPath("pandoc")
	if err != nil {
		t.Skip("pandoc is not installed")
	}
	pages := map[string]string{"rptop": HelpText}
	for v, text := range verbHelp {
		pages["rptop-"+v] = text
	}
	for name, text := range pages {
		var page, errOut bytes.Buffer
		helpPage(&page, text)
		cmd := exec.Command(pandoc, "--from", "markdown", "--to", "man", "-s")
		cmd.Stdin, cmd.Stderr = &page, &errOut
		if err := cmd.Run(); err != nil {
			t.Errorf("%s: pandoc failed: %v\n%s", name, err, errOut.String())
			continue
		}
		if errOut.Len() != 0 {
			t.Errorf("%s: pandoc warned:\n%s", name, errOut.String())
		}
	}
}

// The main help page lists every verb that has a page of its own, so a verb
// added later cannot be left out of `rptop -help` (record once was).
func TestMainHelpListsEveryVerb(t *testing.T) {
	for v := range verbHelp {
		if !strings.Contains(HelpText, "\n"+v+"\n: ") {
			t.Errorf("the main help page does not list the %q verb", v)
		}
	}
}
