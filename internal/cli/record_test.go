package cli

import (
	"bufio"
	"bytes"
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/rsdoiel/rptop/internal/recorder"
	"github.com/rsdoiel/rptop/internal/testenv"
)

func fileLines(t *testing.T, path string) []string {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var out []string
	sc := bufio.NewScanner(bytes.NewReader(b))
	sc.Buffer(nil, 1<<20)
	for sc.Scan() {
		out = append(out, sc.Text())
	}
	return out
}

// recordEnv is an Env whose ticks and cancellation the test drives.
func recordEnv(t *testing.T) (Env, chan time.Time, context.CancelFunc) {
	t.Helper()
	f := testenv.New(t, fixtureDir)
	ticks := make(chan time.Time)
	ctx, cancel := context.WithCancel(context.Background())
	env := Env{Env: f.Env, Ticks: ticks, Context: func() (context.Context, context.CancelFunc) { return ctx, cancel }}
	t.Cleanup(cancel)
	return env, ticks, cancel
}

func runAsync(env Env, args ...string) (done chan int, out, errOut *bytes.Buffer) {
	out, errOut = &bytes.Buffer{}, &bytes.Buffer{}
	done = make(chan int, 1)
	go func() { done <- RunEnv(env, args, out, errOut) }()
	return
}

func waitFile(t *testing.T, dir string) string {
	t.Helper()
	for i := 0; i < 200; i++ {
		if m, _ := filepath.Glob(filepath.Join(dir, "*.jsonl")); len(m) == 1 {
			if b, _ := os.ReadFile(m[0]); strings.Contains(string(b), "\n") {
				return m[0]
			}
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("no recording file appeared")
	return ""
}

func TestRecordUntilInterrupted(t *testing.T) {
	env, ticks, cancel := recordEnv(t)
	dir := t.TempDir()
	done, out, errOut := runAsync(env, "record", "-out", dir, "-label", "before", "-interval", "500ms")
	path := waitFile(t, dir)
	base := time.Date(2026, 11, 15, 14, 30, 12, 0, time.UTC)
	for i := 1; i <= 3; i++ {
		ticks <- base.Add(time.Duration(i) * 500 * time.Millisecond)
	}
	cancel() // Ctrl-C
	select {
	case code := <-done:
		if code != ExitOK {
			t.Fatalf("exit %d, stderr %q", code, errOut.String())
		}
	case <-time.After(5 * time.Second):
		t.Fatal("record did not stop")
	}
	if strings.TrimSpace(out.String()) != path {
		t.Errorf("stdout = %q, want the file path %q", out.String(), path)
	}
	if !strings.Contains(errOut.String(), "recording to "+path) {
		t.Errorf("stderr = %q", errOut.String())
	}
	ls := fileLines(t, path)
	if len(ls) != 5 {
		t.Fatalf("%d lines:\n%s", len(ls), strings.Join(ls, "\n"))
	}
	if !strings.Contains(ls[0], `"type":"meta"`) || !strings.Contains(ls[0], `"label":"before"`) || !strings.Contains(ls[0], `"interval_ms":500`) {
		t.Errorf("meta: %s", ls[0])
	}
	for i := 1; i <= 3; i++ {
		if !strings.Contains(ls[i], `"type":"sample"`) || !strings.Contains(ls[i], `"label":"before"`) {
			t.Errorf("line %d: %s", i, ls[i])
		}
	}
	if !strings.Contains(ls[4], `"type":"meta_end"`) || !strings.Contains(ls[4], `"samples":3`) || !strings.Contains(ls[4], `"reason":"interrupt","clean":true`) {
		t.Errorf("meta_end: %s", ls[4])
	}
}

func TestRecordStopsAtDuration(t *testing.T) {
	f := testenv.New(t, fixtureDir)
	env := Env{Env: f.Env} // real ticker and no interrupt: only -duration ends it
	dir := t.TempDir()
	start := time.Now()
	code, out, errOut := run(env, "record", "-out", dir, "-interval", "500ms", "-duration", "1200ms")
	if code != ExitOK {
		t.Fatalf("exit %d, stderr %q", code, errOut)
	}
	if d := time.Since(start); d < 1100*time.Millisecond || d > 3*time.Second {
		t.Errorf("took %v for -duration 1200ms", d)
	}
	ls := fileLines(t, strings.TrimSpace(out))
	last := ls[len(ls)-1]
	if !strings.Contains(last, `"reason":"duration","clean":true`) {
		t.Errorf("meta_end: %s", last)
	}
	if n := len(ls) - 2; n < 1 || n > 3 {
		t.Errorf("%d samples in 1.2 s at 500 ms", n)
	}
}

func TestRecordExistingFileIsLeftAlone(t *testing.T) {
	f := testenv.New(t, fixtureDir)
	env := Env{Env: f.Env}
	p := filepath.Join(t.TempDir(), "mine.jsonl")
	os.WriteFile(p, []byte("precious\n"), 0o644)
	code, out, errOut := run(env, "record", "-out", p, "-duration", "1s")
	if code != ExitCantCreate {
		t.Errorf("exit %d, want 73", code)
	}
	if out != "" || !strings.HasPrefix(errOut, "rptop: record: ") || !strings.Contains(errOut, "file exists") {
		t.Errorf("stdout %q stderr %q", out, errOut)
	}
	if b, _ := os.ReadFile(p); string(b) != "precious\n" {
		t.Errorf("file modified: %q", b)
	}
}

func TestRecordFreeSpaceGuard(t *testing.T) {
	// Low from the start: nothing is created.
	f := testenv.New(t, fixtureDir)
	env := Env{Env: f.Env}
	env.Recorder.Free = func(string) (uint64, error) { return 1 << 20, nil }
	dir := t.TempDir()
	code, out, errOut := run(env, "record", "-out", dir, "-duration", "1s")
	if code != ExitIO || out != "" || !strings.Contains(errOut, "below the guard") {
		t.Errorf("at start: exit %d out %q err %q", code, out, errOut)
	}
	if m, _ := filepath.Glob(filepath.Join(dir, "*")); len(m) != 0 {
		t.Errorf("created %v", m)
	}

	// Falls below the guard during the run: it ends cleanly with low_space and exits 74.
	env, ticks, _ := recordEnv(t)
	calls := 0
	env.Recorder.Free = func(string) (uint64, error) {
		calls++
		if calls <= 2 { // the start check and the first sample
			return 10 << 30, nil
		}
		return 1 << 20, nil
	}
	dir = t.TempDir()
	done, out2, errOut2 := runAsync(env, "record", "-out", dir, "-interval", "500ms")
	path := waitFile(t, dir)
	base := time.Date(2026, 11, 15, 14, 30, 12, 0, time.UTC)
	ticks <- base.Add(500 * time.Millisecond)
	ticks <- base.Add(1000 * time.Millisecond)
	select {
	case code := <-done:
		if code != ExitIO {
			t.Errorf("exit %d, want 74 (stderr %q)", code, errOut2.String())
		}
	case <-time.After(5 * time.Second):
		t.Fatal("record did not stop on low space")
	}
	ls := fileLines(t, path)
	last := ls[len(ls)-1]
	if !strings.Contains(last, `"reason":"low_space","clean":false`) || !strings.Contains(last, `"samples":1`) {
		t.Errorf("meta_end: %s", last)
	}
	if out2.String() != "" {
		t.Errorf("stdout = %q on a failed run", out2.String())
	}
}

func TestRecordUsageErrors(t *testing.T) {
	f := testenv.New(t, fixtureDir)
	env := Env{Env: f.Env}
	dir := t.TempDir()
	for _, args := range [][]string{
		{"record", "-out", dir, "-interval", "100ms"},
		{"record", "-out", dir, "-interval", "0s"},
		{"record", "-out", dir, "-interval", "-1s"},
		{"record", "-out", dir, "-interval", "soon"},
		{"record", "-out", dir, "-duration", "-1s"},
		{"record", "-out", dir, "-duration", "forever"},
		{"record", "-out", dir, "extra"},
		{"record", "-no-such-flag"},
	} {
		code, out, errOut := run(env, args...)
		if code != ExitUsage || out != "" || !strings.HasPrefix(errOut, "rptop: ") {
			t.Errorf("%v: exit %d out %q err %q", args, code, out, errOut)
		}
	}
	if m, _ := filepath.Glob(filepath.Join(dir, "*")); len(m) != 0 {
		t.Errorf("a usage error created %v", m)
	}
}

// The path a signal takes in production: the real binary, a real SIGINT or
// SIGTERM, and the file it leaves behind (workspace DR-0007).
func TestRecordEndsCleanlyOnRealSignals(t *testing.T) {
	if testing.Short() || runtime.GOOS != "linux" {
		t.Skip("builds and runs the real binary")
	}
	bin := filepath.Join(t.TempDir(), "rptop")
	if out, err := exec.Command("go", "build", "-o", bin, "../../cmd/rptop").CombinedOutput(); err != nil {
		t.Fatalf("build: %v\n%s", err, out)
	}
	for _, sig := range []syscall.Signal{syscall.SIGINT, syscall.SIGTERM} {
		dir := t.TempDir()
		cmd := exec.Command(bin, "record", "-out", dir, "-interval", "500ms", "-label", "sig")
		var stdout, stderr bytes.Buffer
		cmd.Stdout, cmd.Stderr = &stdout, &stderr
		if err := cmd.Start(); err != nil {
			t.Fatal(err)
		}
		path := waitFile(t, dir)
		for i := 0; i < 100 && len(fileLines(t, path)) < 3; i++ { // meta + 2 samples
			time.Sleep(50 * time.Millisecond)
		}
		cmd.Process.Signal(sig)
		waitErr := make(chan error, 1)
		go func() { waitErr <- cmd.Wait() }()
		select {
		case err := <-waitErr:
			if err != nil {
				t.Errorf("%v: exit error %v (stderr %q)", sig, err, stderr.String())
			}
		case <-time.After(10 * time.Second):
			cmd.Process.Kill()
			t.Fatalf("%v: did not exit", sig)
		}
		ls := fileLines(t, path)
		last := ls[len(ls)-1]
		if !strings.Contains(last, `"type":"meta_end"`) || !strings.Contains(last, `"reason":"interrupt","clean":true`) {
			t.Errorf("%v: last line %s", sig, last)
		}
		if len(ls) < 4 {
			t.Errorf("%v: only %d lines", sig, len(ls))
		}
		if strings.TrimSpace(stdout.String()) != path {
			t.Errorf("%v: stdout %q, want %q", sig, stdout.String(), path)
		}
	}
}

func TestRecordHelpAndVerbList(t *testing.T) {
	f := testenv.New(t, fixtureDir)
	env := Env{Env: f.Env}
	code, out, _ := run(env, "record", "-h")
	if code != ExitOK || !strings.Contains(out, "rptop-record(1)") || !strings.Contains(out, "-duration") || !strings.Contains(out, "73") {
		t.Errorf("code %d, page %q", code, out)
	}
	_ = recorder.DefaultMinFree
}
