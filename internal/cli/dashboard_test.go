package cli

import (
	"bytes"
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/rsdoiel/rptop/internal/testenv"
	"github.com/rsdoiel/rptop/internal/ui"
)

// fakeProgram runs a model the way Bubble Tea does, without a terminal: it
// delivers messages one at a time and runs the commands the model returns.
type fakeProgram struct {
	mu      sync.Mutex
	m       tea.Model
	msgs    chan tea.Msg
	started chan struct{}
	done    chan struct{}
	samples int
	ended   int
}

func newFakeProgram(m tea.Model) *fakeProgram {
	p := &fakeProgram{m: m, msgs: make(chan tea.Msg, 64), started: make(chan struct{}), done: make(chan struct{})}
	p.msgs <- tea.WindowSizeMsg{Width: 80, Height: 24} // a real program reports its size first
	return p
}

func (p *fakeProgram) Send(msg tea.Msg) {
	select {
	case p.msgs <- msg:
	case <-p.done:
	}
}

func (p *fakeProgram) Run() error {
	defer close(p.done)
	close(p.started)
	for msg := range p.msgs {
		if _, quit := msg.(tea.QuitMsg); quit {
			return nil
		}
		p.mu.Lock()
		switch msg.(type) {
		case ui.SampleMsg:
			p.samples++
		case ui.RecEndedMsg:
			p.ended++
		}
		next, cmd := p.m.Update(msg)
		p.m = next
		p.mu.Unlock()
		if cmd != nil {
			go func() {
				if out := cmd(); out != nil {
					select {
					case p.msgs <- out:
					case <-p.done:
					}
				}
			}()
		}
	}
	return nil
}

func (p *fakeProgram) screen() string {
	p.mu.Lock()
	defer p.mu.Unlock()
	return ansi.Strip(p.m.View().Content)
}

func (p *fakeProgram) count() (samples, ended int) {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.samples, p.ended
}

func keyMsg(s string) tea.KeyPressMsg {
	if s == "ctrl+c" {
		return tea.KeyPressMsg{Code: 'c', Mod: tea.ModCtrl}
	}
	return tea.KeyPressMsg{Code: []rune(s)[0], Text: s}
}

func eventually(t *testing.T, what string, f func() bool) {
	t.Helper()
	for i := 0; i < 300; i++ {
		if f() {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for %s", what)
}

func TestBareRptopWithoutATerminalPrintsHelpAndExitsTwo(t *testing.T) {
	f := testenv.New(t, fixtureDir)
	for _, env := range []Env{{Env: f.Env}, {Env: f.Env, IsTerminal: func() bool { return false }}} {
		code, out, errOut := run(env)
		if code != ExitUsage {
			t.Errorf("exit %d, want 2", code)
		}
		if out != "" || !strings.Contains(errOut, "rptop(1)") {
			t.Errorf("stdout %q; help should be on stderr: %q", out, errOut)
		}
	}
}

func TestDashboardOptionErrors(t *testing.T) {
	f := testenv.New(t, fixtureDir)
	env := Env{Env: f.Env, IsTerminal: func() bool { return true }}
	for _, args := range [][]string{
		{"-interval", "100ms"},
		{"-i", "0s"},
		{"-interval", "soon"},
		{"-warn-temp", "hot"},
		{"-crit-temp", "x"},
		{"-warn-temp", "80", "-crit-temp", "70"},
		{"-warn-temp", "70", "-crit-temp", "70"},
		{"-crit-temp", "50"}, // below the default warn of 60
		{"-interval", "2s", "temp", "soc"},
		{"-warn-temp", "50", "list"},
		{"-out", "x", "fan"},
	} {
		code, out, errOut := run(env, args...)
		if code != ExitUsage || out != "" || !strings.HasPrefix(errOut, "rptop: ") {
			t.Errorf("%v: exit %d out %q err %q", args, code, out, errOut)
		}
	}
}

// The whole pipeline without a terminal: press r, let samples arrive, press q.
func TestDashboardRecordsAndQuits(t *testing.T) {
	f := testenv.New(t, fixtureDir)
	ticks := make(chan time.Time)
	var prog *fakeProgram
	var pmu sync.Mutex
	env := Env{Env: f.Env, Ticks: ticks, IsTerminal: func() bool { return true }}
	env.NewProgram = func(m tea.Model) Program {
		pmu.Lock()
		defer pmu.Unlock()
		prog = newFakeProgram(m)
		return prog
	}
	dir := t.TempDir()
	done := make(chan int, 1)
	var out, errOut bytes.Buffer
	go func() {
		done <- RunEnv(env, []string{"-out", dir, "-interval", "500ms", "-warn-temp", "55"}, &out, &errOut)
	}()
	get := func() *fakeProgram { pmu.Lock(); defer pmu.Unlock(); return prog }
	eventually(t, "the program to start", func() bool { return get() != nil })
	p := get()
	<-p.started
	base := time.Date(2026, 11, 15, 14, 30, 12, 0, time.UTC)
	ticks <- base.Add(500 * time.Millisecond)
	eventually(t, "the first sample on screen", func() bool { s, _ := p.count(); return s == 1 })
	if v := p.screen(); !strings.Contains(v, "SoC") || !strings.Contains(v, "55.1 °C") {
		t.Errorf("screen:\n%s", v)
	}
	p.Send(keyMsg("r")) // start recording
	eventually(t, "the recording file", func() bool { m, _ := filepath.Glob(filepath.Join(dir, "*.jsonl")); return len(m) == 1 })
	m, _ := filepath.Glob(filepath.Join(dir, "*.jsonl"))
	path := m[0]
	for i := 2; i <= 4; i++ {
		ticks <- base.Add(time.Duration(i) * 500 * time.Millisecond)
	}
	eventually(t, "three samples in the file", func() bool { return len(fileLines(t, path)) >= 4 })
	if v := p.screen(); !strings.Contains(v, "rec ●") || !strings.Contains(v, filepath.Base(path)) {
		t.Errorf("header does not show the recording:\n%s", v)
	}
	p.Send(keyMsg("q"))
	select {
	case code := <-done:
		if code != ExitOK {
			t.Fatalf("exit %d, stderr %q", code, errOut.String())
		}
	case <-time.After(5 * time.Second):
		t.Fatal("the dashboard did not exit")
	}
	ls := fileLines(t, path)
	last := ls[len(ls)-1]
	if !strings.Contains(last, `"type":"meta_end"`) || !strings.Contains(last, `"reason":"stopped","clean":true`) {
		t.Errorf("last line: %s", last)
	}
	if !strings.Contains(ls[1], `"seq":0,`) {
		t.Errorf("recording starts at seq %s", ls[1])
	}
	if out.Len() != 0 {
		t.Errorf("stdout = %q; a dashboard prints nothing", out.String())
	}
}

// A recording that cannot go on does not end the dashboard (DR-0004 7).
func TestDashboardSurvivesALowSpaceRecording(t *testing.T) {
	f := testenv.New(t, fixtureDir)
	ticks := make(chan time.Time)
	var prog *fakeProgram
	var pmu sync.Mutex
	env := Env{Env: f.Env, Ticks: ticks, IsTerminal: func() bool { return true }}
	env.NewProgram = func(m tea.Model) Program {
		pmu.Lock()
		defer pmu.Unlock()
		prog = newFakeProgram(m)
		return prog
	}
	calls := 0
	env.Recorder.Free = func(string) (uint64, error) {
		pmu.Lock()
		defer pmu.Unlock()
		calls++
		if calls <= 2 { // the start check and the first sample
			return 10 << 30, nil
		}
		return 1 << 20, nil
	}
	dir := t.TempDir()
	done := make(chan int, 1)
	var out, errOut bytes.Buffer
	go func() { done <- RunEnv(env, []string{"-out", dir, "-interval", "500ms"}, &out, &errOut) }()
	eventually(t, "the program", func() bool { pmu.Lock(); defer pmu.Unlock(); return prog != nil })
	pmu.Lock()
	p := prog
	pmu.Unlock()
	<-p.started
	p.Send(keyMsg("r"))
	eventually(t, "the recording file", func() bool { m, _ := filepath.Glob(filepath.Join(dir, "*.jsonl")); return len(m) == 1 })
	base := time.Date(2026, 11, 15, 14, 30, 12, 0, time.UTC)
	ticks <- base.Add(500 * time.Millisecond)
	ticks <- base.Add(1000 * time.Millisecond) // the guard ends the run here
	eventually(t, "the banner", func() bool { _, e := p.count(); return e == 1 })
	ticks <- base.Add(1500 * time.Millisecond) // the dashboard still gets samples
	eventually(t, "a sample after the end", func() bool { s, _ := p.count(); return s == 3 })
	if v := p.screen(); !strings.Contains(v, "recording ended") || !strings.Contains(v, "low_space") {
		t.Errorf("no banner:\n%s", v)
	}
	p.Send(keyMsg("q"))
	select {
	case code := <-done:
		if code != ExitOK {
			t.Errorf("exit %d, want 0: the dashboard is not the recording", code)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("did not exit")
	}
	m, _ := filepath.Glob(filepath.Join(dir, "*.jsonl"))
	if ls := fileLines(t, m[0]); !strings.Contains(ls[len(ls)-1], `"reason":"low_space","clean":false`) {
		t.Errorf("last line: %s", ls[len(ls)-1])
	}
}

// SIGTERM (the Context) ends the dashboard cleanly and ends an open recording.
func TestDashboardEndsOnTheContext(t *testing.T) {
	f := testenv.New(t, fixtureDir)
	ticks := make(chan time.Time)
	var prog *fakeProgram
	var pmu sync.Mutex
	ctx, cancel := context.WithCancel(context.Background())
	env := Env{Env: f.Env, Ticks: ticks, IsTerminal: func() bool { return true },
		Context: func() (context.Context, context.CancelFunc) { return ctx, cancel }}
	env.NewProgram = func(m tea.Model) Program {
		pmu.Lock()
		defer pmu.Unlock()
		prog = newFakeProgram(m)
		return prog
	}
	dir := t.TempDir()
	done := make(chan int, 1)
	go func() { done <- RunEnv(env, []string{"-out", dir}, &bytes.Buffer{}, &bytes.Buffer{}) }()
	eventually(t, "the program", func() bool { pmu.Lock(); defer pmu.Unlock(); return prog != nil })
	pmu.Lock()
	p := prog
	pmu.Unlock()
	<-p.started
	p.Send(keyMsg("r"))
	eventually(t, "the recording file", func() bool { m, _ := filepath.Glob(filepath.Join(dir, "*.jsonl")); return len(m) == 1 })
	cancel() // SIGTERM
	select {
	case code := <-done:
		if code != ExitOK {
			t.Errorf("exit %d", code)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("did not exit on the context")
	}
	m, _ := filepath.Glob(filepath.Join(dir, "*.jsonl"))
	if b, _ := os.ReadFile(m[0]); !strings.Contains(string(b), `"type":"meta_end"`) {
		t.Errorf("the open recording was left without a meta_end:\n%s", b)
	}
}

// /dev/null is a character device but not a terminal: a script that redirects
// to it must get the exit 2 and the help, not a failed attempt to open a TTY.
func TestDevNullIsNotATerminal(t *testing.T) {
	null, err := os.OpenFile(os.DevNull, os.O_RDWR, 0)
	if err != nil {
		t.Skip(err)
	}
	defer null.Close()
	in, out := os.Stdin, os.Stdout
	os.Stdin, os.Stdout = null, null
	defer func() { os.Stdin, os.Stdout = in, out }()
	if stdioIsTerminal() {
		t.Error("/dev/null counted as a terminal")
	}
}

// The same through the real binary, as a script would run it.
func TestRealBinaryWithoutATerminalExitsTwo(t *testing.T) {
	if testing.Short() || runtime.GOOS != "linux" {
		t.Skip("builds and runs the real binary")
	}
	bin := filepath.Join(t.TempDir(), "rptop")
	if out, err := exec.Command("go", "build", "-o", bin, "../../cmd/rptop").CombinedOutput(); err != nil {
		t.Fatalf("build: %v\n%s", err, out)
	}
	null, _ := os.OpenFile(os.DevNull, os.O_RDWR, 0)
	defer null.Close()
	var errOut bytes.Buffer
	cmd := exec.Command(bin)
	cmd.Stdin, cmd.Stdout, cmd.Stderr = null, null, &errOut
	err := cmd.Run()
	var ee *exec.ExitError
	if !errors.As(err, &ee) || ee.ExitCode() != ExitUsage {
		t.Errorf("exit = %v, want 2 (stderr %q)", err, errOut.String())
	}
	if !strings.Contains(errOut.String(), "rptop(1)") {
		t.Errorf("stderr has no help: %q", errOut.String())
	}
}
