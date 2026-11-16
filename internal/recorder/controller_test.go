package recorder

import (
	"bufio"
	"bytes"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/rsdoiel/rptop/internal/exitcode"
	"github.com/rsdoiel/rptop/internal/sample"
)

var t0 = time.Date(2026, 11, 15, 14, 30, 12, 0, time.UTC)

type clock struct {
	mu sync.Mutex
	t  time.Time
}

func (c *clock) now() time.Time  { c.mu.Lock(); defer c.mu.Unlock(); return c.t }
func (c *clock) set(t time.Time) { c.mu.Lock(); c.t = t; c.mu.Unlock() }

func testMeta(now time.Time) sample.Meta {
	return sample.Meta{TS: now, Rptop: "0.0.0 test", IntervalMS: 1000,
		Elements: []sample.ElementInfo{{Name: "temp.soc", Unit: "°C", Available: true}}}
}

func smp(seq int, secs int) sample.Sample {
	return sample.Sample{Run: "ignored", Seq: 99, TS: t0.Add(time.Duration(secs) * time.Second), DtMS: 1000,
		Entries: []sample.Entry{{Name: "temp.soc", Value: sample.Num(54)}}}
}

func newController(t *testing.T, mutate func(*Options)) (*Controller, *clock, string) {
	t.Helper()
	dir := t.TempDir()
	clk := &clock{t: t0}
	o := Options{Dir: dir, Now: clk.now, Free: func(string) (uint64, error) { return 10 << 30, nil }}
	if mutate != nil {
		mutate(&o)
	}
	return NewController(testMeta, o), clk, dir
}

func lines(t *testing.T, path string) []string {
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

func TestRecordingLayout(t *testing.T) {
	c, clk, dir := newController(t, nil)
	st, err := c.Start("before-ollama", "")
	if err != nil {
		t.Fatal(err)
	}
	if want := filepath.Join(dir, "rptop-20261115-143012.jsonl"); st.Path != want {
		t.Errorf("default path = %s, want %s", st.Path, want)
	}
	if !st.Recording || st.Run != "20261115-143012" || st.Label != "before-ollama" {
		t.Errorf("status = %+v", st)
	}
	for i := 1; i <= 3; i++ {
		if err := c.Feed(smp(0, i)); err != nil {
			t.Fatal(err)
		}
	}
	clk.set(t0.Add(3500 * time.Millisecond))
	end, err := c.Stop(sample.ReasonInterrupt)
	if err != nil {
		t.Fatal(err)
	}
	if end.Recording || end.Samples != 3 || end.Reason != sample.ReasonInterrupt {
		t.Errorf("end status = %+v", end)
	}
	ls := lines(t, st.Path)
	if len(ls) != 5 {
		t.Fatalf("%d lines, want meta + 3 samples + meta_end:\n%s", len(ls), strings.Join(ls, "\n"))
	}
	wantTypes := []string{"meta", "sample", "sample", "sample", "meta_end"}
	for i, l := range ls {
		p, err := sample.Parse([]byte(l))
		if err != nil || p.Type != wantTypes[i] {
			t.Errorf("line %d: %v %q (want %s)", i, err, l, wantTypes[i])
		}
		if strings.Contains(l, `"ignored"`) {
			t.Errorf("line %d kept the feeder's run: %s", i, l)
		}
	}
	// seq is renumbered from 0 with no gaps, whatever the feeder said.
	for i := 1; i <= 3; i++ {
		if !strings.Contains(ls[i], fmt.Sprintf(`"seq":%d,`, i-1)) {
			t.Errorf("line %d seq: %s", i, ls[i])
		}
		if !strings.Contains(ls[i], `"run":"20261115-143012","label":"before-ollama"`) {
			t.Errorf("line %d run/label: %s", i, ls[i])
		}
	}
	wantEnd := `{"type":"meta_end","run":"20261115-143012","label":"before-ollama","ts":"2026-11-15T14:30:15.500Z","samples":3,"duration_ms":3500,"reason":"interrupt","clean":true}`
	if ls[4] != wantEnd {
		t.Errorf("meta_end:\n got %s\nwant %s", ls[4], wantEnd)
	}
	if !strings.HasPrefix(ls[0], `{"type":"meta","schema":1,"run":"20261115-143012","label":"before-ollama"`) {
		t.Errorf("meta: %s", ls[0])
	}
}

func TestOutDirectoryAndNewDirectory(t *testing.T) {
	c, _, dir := newController(t, nil)
	st, err := c.Start("", dir) // an existing directory: default name inside it
	if err != nil || st.Path != filepath.Join(dir, "rptop-20261115-143012.jsonl") {
		t.Fatalf("dir as -out: %v %+v", err, st)
	}
	c.Stop(sample.ReasonStopped)
	nested := filepath.Join(dir, "runs", "a", "x.jsonl")
	st, err = c.Start("", nested)
	if err != nil || st.Path != nested {
		t.Fatalf("new directory: %v %+v", err, st)
	}
	c.Stop(sample.ReasonStopped)
	if _, err := os.Stat(nested); err != nil {
		t.Errorf("file not created: %v", err)
	}
}

// DR-0002 9: 73 when the file exists or its directory cannot be made.
func TestCannotCreate(t *testing.T) {
	c, _, dir := newController(t, nil)
	existing := filepath.Join(dir, "mine.jsonl")
	os.WriteFile(existing, []byte("precious\n"), 0o644)
	_, err := c.Start("", existing)
	if exitcode.Code(err) != exitcode.CantCreate {
		t.Errorf("existing file: err = %v, code %d, want 73", err, exitcode.Code(err))
	}
	if b, _ := os.ReadFile(existing); string(b) != "precious\n" {
		t.Errorf("existing file was modified: %q", b)
	}
	file := filepath.Join(dir, "afile")
	os.WriteFile(file, nil, 0o644)
	_, err = c.Start("", filepath.Join(file, "sub", "x.jsonl")) // a file where a directory must be
	if exitcode.Code(err) != exitcode.CantCreate {
		t.Errorf("directory cannot be made: err = %v, code %d, want 73", err, exitcode.Code(err))
	}
	if c.Status().Recording {
		t.Error("recording after a failed Start")
	}
}

func TestStartWhileRecordingIsRejected(t *testing.T) {
	c, _, _ := newController(t, nil)
	if _, err := c.Start("a", ""); err != nil {
		t.Fatal(err)
	}
	if _, err := c.Start("b", ""); !errors.Is(err, exitcode.ErrUsage) {
		t.Errorf("second Start: %v, want a usage error", err)
	}
	c.Stop(sample.ReasonStopped)
}

// DR-0003 8: below the guard the run ends cleanly with low_space and exit 74.
func TestLowSpaceStopsTheRunWithMetaEnd(t *testing.T) {
	var free uint64 = 10 << 30
	c, _, _ := newController(t, func(o *Options) {
		o.Free = func(string) (uint64, error) { return free, nil }
	})
	st, _ := c.Start("", "")
	if err := c.Feed(smp(0, 1)); err != nil {
		t.Fatal(err)
	}
	free = 63 << 20 // just under 64 MiB
	err := c.Feed(smp(0, 2))
	if exitcode.Code(err) != exitcode.IO || !errors.Is(err, ErrLowSpace) {
		t.Errorf("err = %v, code %d, want ErrLowSpace, 74", err, exitcode.Code(err))
	}
	if c.Status().Recording || c.Status().Reason != sample.ReasonLowSpace {
		t.Errorf("status = %+v", c.Status())
	}
	ls := lines(t, st.Path)
	last := ls[len(ls)-1]
	if !strings.Contains(last, `"type":"meta_end"`) || !strings.Contains(last, `"reason":"low_space","clean":false`) || !strings.Contains(last, `"samples":1`) {
		t.Errorf("last line: %s", last)
	}
	if len(ls) != 3 {
		t.Errorf("%d lines; the refused sample must not be written", len(ls))
	}
	if err := c.Feed(smp(0, 3)); err != nil { // after the end, feeding is a no-op
		t.Errorf("Feed after the end: %v", err)
	}
}

func TestLowSpaceAtStartCreatesNothing(t *testing.T) {
	c, _, dir := newController(t, func(o *Options) {
		o.Free = func(string) (uint64, error) { return 1 << 20, nil }
	})
	_, err := c.Start("", "")
	if exitcode.Code(err) != exitcode.IO {
		t.Errorf("err = %v, code %d, want 74", err, exitcode.Code(err))
	}
	if m, _ := filepath.Glob(filepath.Join(dir, "*.jsonl")); len(m) != 0 {
		t.Errorf("created %v", m)
	}
}

// fakeFile fails a chosen write and records syncs.
type fakeFile struct {
	buf     bytes.Buffer
	writes  int
	failAt  int // 1-based write number that fails; 0 never
	syncs   int
	closed  bool
	failAll bool
}

func (f *fakeFile) Write(p []byte) (int, error) {
	f.writes++
	if f.failAll || f.writes == f.failAt {
		return 0, errors.New("no space left on device")
	}
	return f.buf.Write(p)
}
func (f *fakeFile) Sync() error  { f.syncs++; return nil }
func (f *fakeFile) Close() error { f.closed = true; return nil }

// DR-0003 8: a failed write tries once to write meta_end, and the run is 74.
func TestWriteErrorTriesMetaEnd(t *testing.T) {
	ff := &fakeFile{failAt: 3} // meta, one sample, then the failing sample
	c, _, _ := newController(t, func(o *Options) {
		o.Create = func(string) (File, error) { return ff, nil }
	})
	c.Start("", "")
	if err := c.Feed(smp(0, 1)); err != nil {
		t.Fatal(err)
	}
	err := c.Feed(smp(0, 2))
	if exitcode.Code(err) != exitcode.IO {
		t.Errorf("err = %v, code %d, want 74", err, exitcode.Code(err))
	}
	ls := strings.Split(strings.TrimSpace(ff.buf.String()), "\n")
	last := ls[len(ls)-1]
	if !strings.Contains(last, `"type":"meta_end"`) || !strings.Contains(last, `"reason":"write_error","clean":false`) {
		t.Errorf("last line: %s", last)
	}
	if !ff.closed || c.Status().Recording {
		t.Errorf("closed %v, status %+v", ff.closed, c.Status())
	}
}

func TestWriteErrorWhenMetaEndAlsoFailsStillEnds(t *testing.T) {
	ff := &fakeFile{failAll: true}
	c, _, _ := newController(t, func(o *Options) {
		o.Create = func(string) (File, error) { return ff, nil }
	})
	_, err := c.Start("", "") // the meta line cannot be written
	if exitcode.Code(err) != exitcode.IO {
		t.Errorf("Start: err = %v, code %d, want 74", err, exitcode.Code(err))
	}
	if !ff.closed || c.Status().Recording {
		t.Errorf("closed %v, status %+v", ff.closed, c.Status())
	}
}

func TestSyncAtLeastEveryTenSecondsAndAtTheEnd(t *testing.T) {
	ff := &fakeFile{}
	c, clk, _ := newController(t, func(o *Options) {
		o.Create = func(string) (File, error) { return ff, nil }
	})
	c.Start("", "")
	base := ff.syncs
	for i := 1; i <= 25; i++ {
		clk.set(t0.Add(time.Duration(i) * time.Second))
		c.Feed(smp(0, i))
	}
	if got := ff.syncs - base; got < 2 || got > 3 {
		t.Errorf("%d syncs over 25 s, want about every 10 s", got)
	}
	before := ff.syncs
	c.Stop(sample.ReasonDuration)
	if ff.syncs != before+1 {
		t.Errorf("no sync at meta_end (%d -> %d)", before, ff.syncs)
	}
	if ls := strings.Split(strings.TrimSpace(ff.buf.String()), "\n"); len(ls) != 27 {
		t.Errorf("%d lines, want 27", len(ls))
	}
}

func TestMarker(t *testing.T) {
	c, _, _ := newController(t, nil)
	if err := c.Marker("too early"); !errors.Is(err, exitcode.ErrUsage) {
		t.Errorf("Marker while idle: %v, want a usage error", err)
	}
	st, _ := c.Start("run", "")
	c.Feed(smp(0, 1))
	c.Feed(smp(0, 2))
	if err := c.Marker("before benchmark"); err != nil {
		t.Fatal(err)
	}
	c.Stop(sample.ReasonStopped)
	ls := lines(t, st.Path)
	m := ls[3]
	if !strings.Contains(m, `"type":"marker"`) || !strings.Contains(m, `"seq":1,"text":"before benchmark"`) {
		t.Errorf("marker line: %s", m)
	}
	if len(ls) != 5 {
		t.Errorf("%d lines", len(ls))
	}
}

func TestStopIsIdempotentAndStatusTracksTheRun(t *testing.T) {
	c, _, _ := newController(t, nil)
	if st := c.Status(); st.Recording {
		t.Errorf("recording before Start: %+v", st)
	}
	if _, err := c.Stop(sample.ReasonStopped); err != nil {
		t.Errorf("Stop while idle: %v", err)
	}
	c.Start("x", "")
	c.Feed(smp(0, 1))
	if st := c.Status(); !st.Recording || st.Samples != 1 {
		t.Errorf("status = %+v", st)
	}
	c.Stop(sample.ReasonStopped)
	st, err := c.Stop(sample.ReasonStopped)
	if err != nil || st.Samples != 1 || st.Reason != sample.ReasonStopped {
		t.Errorf("second Stop: %v %+v", err, st)
	}
	// A new run after the first has ended starts a clean file and seq from 0.
	if _, err := c.Start("y", filepath.Join(t.TempDir(), "second.jsonl")); err != nil {
		t.Fatal(err)
	}
	c.Feed(smp(0, 1))
	if st := c.Status(); st.Samples != 1 || st.Label != "y" {
		t.Errorf("second run status = %+v", st)
	}
	c.Stop(sample.ReasonStopped)
}

var stamp = regexp.MustCompile(`^rptop-\d{8}-\d{6}\.jsonl$`)

func TestDefaultNameIsUTC(t *testing.T) {
	local := time.FixedZone("far", 9*3600)
	c, _, _ := newController(t, func(o *Options) {
		o.Now = func() time.Time { return time.Date(2026, 11, 16, 1, 30, 12, 0, local) } // 16:30:12 the day before in UTC
	})
	st, err := c.Start("", "")
	if err != nil {
		t.Fatal(err)
	}
	c.Stop(sample.ReasonStopped)
	if base := filepath.Base(st.Path); !stamp.MatchString(base) || base != "rptop-20261115-163012.jsonl" {
		t.Errorf("name = %s", base)
	}
}

// A path ending in a separator names a directory, made if it does not exist.
func TestTrailingSeparatorNamesANewDirectory(t *testing.T) {
	c, _, dir := newController(t, nil)
	for _, p := range []string{filepath.Join(dir, "runs") + "/", filepath.Join(dir, "a", "b") + "/"} {
		st, err := c.Start("", p)
		if err != nil {
			t.Fatalf("%s: %v", p, err)
		}
		if want := filepath.Join(strings.TrimRight(p, "/"), "rptop-20261115-143012.jsonl"); st.Path != want {
			t.Errorf("path = %s, want %s", st.Path, want)
		}
		if _, err := os.Stat(st.Path); err != nil {
			t.Errorf("file not created: %v", err)
		}
		c.Stop(sample.ReasonStopped)
	}
}
