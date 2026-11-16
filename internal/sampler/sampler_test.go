package sampler

import (
	"context"
	"errors"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/rsdoiel/rptop/internal/collector"
	"github.com/rsdoiel/rptop/internal/exitcode"
	"github.com/rsdoiel/rptop/internal/sample"
	"github.com/rsdoiel/rptop/internal/testenv"
)

const fixtureDir = "../../testdata/pi5-trixie"

var start = time.Date(2026, 11, 15, 14, 30, 12, 0, time.UTC)

type harness struct {
	t     *testing.T
	f     *testenv.Fixture
	s     *Sampler
	ticks chan time.Time
	out   chan sample.Sample
	done  chan error
	stop  context.CancelFunc
}

// begin starts a sampler on the fixtures and waits for its baseline.
func begin(t *testing.T, mutate func(*Config)) *harness {
	t.Helper()
	f := testenv.New(t, fixtureDir)
	cfg := Config{Env: f.Env, Run: "20261115-143012", Interval: time.Second, Timeout: 200 * time.Millisecond}
	h := &harness{t: t, f: f, ticks: make(chan time.Time), out: make(chan sample.Sample, 16), done: make(chan error, 1)}
	cfg.Ticks = h.ticks
	cfg.Now = func() time.Time { return start }
	if mutate != nil {
		mutate(&cfg)
	}
	s, err := New(cfg)
	if err != nil {
		t.Fatal(err)
	}
	h.s = s
	ctx, cancel := context.WithCancel(context.Background())
	h.stop = cancel
	go func() { h.done <- s.Run(ctx, func(x sample.Sample) { h.out <- x }) }()
	select {
	case <-s.Ready():
	case <-time.After(5 * time.Second):
		t.Fatal("sampler never took its baseline")
	}
	t.Cleanup(func() { cancel() })
	return h
}

// tick fires one tick n seconds after the start and returns the sample.
func (h *harness) tick(n int) sample.Sample {
	h.t.Helper()
	h.ticks <- start.Add(time.Duration(n) * time.Second)
	select {
	case s := <-h.out:
		return s
	case <-time.After(5 * time.Second):
		h.t.Fatal("no sample after a tick")
	}
	return sample.Sample{}
}

func get(s sample.Sample, name string) (float64, bool) {
	for _, e := range s.Entries {
		if e.Name == name {
			if e.Value == nil {
				return 0, false
			}
			return *e.Value, true
		}
	}
	panic("no entry " + name)
}

func want(t *testing.T, s sample.Sample, name string, v float64) {
	t.Helper()
	got, ok := get(s, name)
	if !ok {
		t.Errorf("%s is null, want %v", name, v)
		return
	}
	if d := got - v; d > 1e-6 || d < -1e-6 {
		t.Errorf("%s = %v, want %v", name, got, v)
	}
}

func wantNull(t *testing.T, s sample.Sample, name string) {
	t.Helper()
	if v, ok := get(s, name); ok {
		t.Errorf("%s = %v, want null", name, v)
	}
}

func TestSampleCarriesEveryElement(t *testing.T) {
	h := begin(t, nil)
	h.f.Advance()
	s := h.tick(1)
	if s.Run != "20261115-143012" || s.Seq != 0 || s.DtMS != 1000 || !s.TS.Equal(start.Add(time.Second)) {
		t.Errorf("header = %+v", s)
	}
	want(t, s, "temp.soc", 55.1)
	want(t, s, "temp.pmic", 54.3)
	want(t, s, "temp.rp1", 55.472)
	wantNull(t, s, "temp.hailo") // absent: null, never 0
	want(t, s, "cpu.total", 50)
	want(t, s, "cpu.core1", 30)
	want(t, s, "cpu.freq", 1800)
	want(t, s, "mem.total", 17005985792)
	want(t, s, "fan.speed", 3725)
	want(t, s, "volts.core", 0.7838)
	want(t, s, "disk.read", 1048576)
	want(t, s, "disk.write", 0)
	want(t, s, "net.rx", 1000)
	want(t, s, "throttle.now", 0)
	want(t, s, "throttle.since_boot", 14) // 0xe0000 >> 16
	want(t, s, "self.cpu", 0)
	want(t, s, "self.rss", 18432*1024)
}

// DR-0003 5: the keys and their order are fixed; the Hailo key is present as null.
func TestEntryOrderMatchesRptopList(t *testing.T) {
	h := begin(t, nil)
	h.f.Advance()
	s := h.tick(1)
	var names []string
	for _, e := range s.Entries {
		names = append(names, e.Name)
	}
	got := strings.Join(names, " ")
	wantPrefix := "temp.soc temp.pmic temp.hailo temp.rp1 cpu.total cpu.core0 cpu.core1 cpu.core2 cpu.core3 cpu.freq cpu.load " +
		"mem.used mem.total mem.swap fan.speed volts.core disk.read disk.write net.rx net.tx"
	if !strings.HasPrefix(got, wantPrefix) || !strings.HasSuffix(got, "throttle.now throttle.since_boot self.cpu self.rss") {
		t.Errorf("entries:\n%s", got)
	}
}

func TestSeqHasNoGapsAndDtIsMeasured(t *testing.T) {
	h := begin(t, nil)
	var last int = -1
	for i, secs := range []int{1, 2, 4} {
		h.f.Advance()
		s := h.tick(secs)
		if s.Seq != i || s.Seq != last+1 {
			t.Errorf("seq = %d, want %d", s.Seq, i)
		}
		last = s.Seq
		wantDt := map[int]int64{0: 1000, 1: 1000, 2: 2000}[i]
		if s.DtMS != wantDt {
			t.Errorf("tick %d: dt_ms = %d, want %d", i, s.DtMS, wantDt)
		}
	}
}

// A rate with nothing to compare (the counters did not move) is null, not an error that stops the run.
func TestStalledCounterIsNullAndRunContinues(t *testing.T) {
	h := begin(t, nil)
	s := h.tick(1) // no Advance: cpu counters identical
	wantNull(t, s, "cpu.total")
	want(t, s, "temp.soc", 55.1)
	h.f.Advance()
	want(t, h.tick(2), "cpu.total", 50)
}

type blockingHailo struct {
	release chan struct{}
	calls   atomic.Int32
}

func (b *blockingHailo) Temps() (collector.HailoTemps, error) {
	b.calls.Add(1)
	<-b.release
	return collector.HailoTemps{TS0: 54.3, TS1: 53.9}, nil
}
func (b *blockingHailo) Close() error { return nil }

// Plan phase 4: a slow collector never delays the tick.
func TestSlowCollectorNeverDelaysTheTick(t *testing.T) {
	b := &blockingHailo{release: make(chan struct{})}
	h := begin(t, func(c *Config) {
		c.Env.OpenHailo = func() (collector.HailoReader, error) { return b, nil }
		c.Timeout = 50 * time.Millisecond
	})
	h.f.Advance()
	t0 := time.Now()
	s := h.tick(1)
	if d := time.Since(t0); d > 2*time.Second {
		t.Errorf("tick took %v with a hung collector", d)
	}
	wantNull(t, s, "temp.hailo")
	want(t, s, "temp.soc", 55.1) // everything else is still there
	// Still hung on the next tick: no second call piles up behind the first.
	h.f.Advance()
	s = h.tick(2)
	wantNull(t, s, "temp.hailo")
	if n := b.calls.Load(); n != 1 {
		t.Errorf("hung collector called %d times, want 1", n)
	}
	close(b.release)
	time.Sleep(50 * time.Millisecond) // let the abandoned read finish
	h.f.Advance()
	want(t, h.tick(3), "temp.hailo", 54.3)
}

func TestSlowSourcesArePolledLessOften(t *testing.T) {
	var mu sync.Mutex
	calls := map[string]int{}
	h := begin(t, func(c *Config) {
		inner := c.Env.Sources.Vcgencmd
		c.Env.Sources.Vcgencmd = func(args ...string) ([]byte, error) {
			mu.Lock()
			calls[strings.Join(args, " ")]++
			mu.Unlock()
			return inner(args...)
		}
	})
	var s sample.Sample
	for i := 1; i <= 4; i++ {
		h.f.Advance()
		s = h.tick(i)
	}
	count := func(k string) int {
		mu.Lock()
		defer mu.Unlock()
		return calls[k]
	}
	if count("measure_temp pmic") != 1 || count("measure_volts core") != 1 {
		t.Errorf("pmic %d, volts %d calls over 4 ticks, want 1 each (cached for 5 s)",
			count("measure_temp pmic"), count("measure_volts core"))
	}
	if count("get_throttled") != 4 {
		t.Errorf("get_throttled called %d times, want every tick (4)", count("get_throttled"))
	}
	want(t, s, "temp.pmic", 54.3) // the cached value is still reported
	want(t, s, "volts.core", 0.7838)
	h.f.Advance()
	h.tick(6)
	if n := count("measure_temp pmic"); n != 2 {
		t.Errorf("pmic calls = %d after 5 s, want 2", n)
	}
}

func TestMetaDescribesEveryKeyOfASample(t *testing.T) {
	h := begin(t, nil)
	m := h.s.Meta(start)
	if m.Run != "20261115-143012" || m.IntervalMS != 1000 || !m.TS.Equal(start) {
		t.Errorf("meta = %+v", m)
	}
	h.f.Advance()
	s := h.tick(1)
	info := map[string]sample.ElementInfo{}
	for _, e := range m.Elements {
		info[e.Name] = e
	}
	for _, e := range s.Entries {
		if _, ok := info[e.Name]; !ok {
			t.Errorf("sample key %s is not in the meta elements", e.Name)
		}
	}
	if len(m.Elements) != len(s.Entries) {
		t.Errorf("meta lists %d elements, a sample has %d keys", len(m.Elements), len(s.Entries))
	}
	hailo := info["temp.hailo"]
	if hailo.Available || hailo.Class != "no_input" || hailo.Unit != "°C" {
		t.Errorf("temp.hailo = %+v, want unavailable, no_input", hailo)
	}
	if soc := info["temp.soc"]; !soc.Available || soc.Class != "" {
		t.Errorf("temp.soc = %+v", soc)
	}
	if info["cpu.core3"].Unit != "%" || info["net.rx"].Unit != "B/s" || info["self.rss"].Unit != "bytes" {
		t.Errorf("units: %+v %+v %+v", info["cpu.core3"], info["net.rx"], info["self.rss"])
	}
}

func TestMetaMarksAnUnreachableDevice(t *testing.T) {
	h := begin(t, func(c *Config) {
		c.Env.OpenHailo = func() (collector.HailoReader, error) { return nil, collector.ErrUnreachable }
	})
	for _, e := range h.s.Meta(start).Elements {
		if e.Name == "temp.hailo" && (e.Available || e.Class != exitcodeClass(collector.ErrUnreachable)) {
			t.Errorf("temp.hailo = %+v, want class unavailable", e)
		}
	}
}

func exitcodeClass(err error) string { return exitcode.Class(exitcode.Code(err)) }

func TestNothingIsSampledBeforeTheFirstTick(t *testing.T) {
	h := begin(t, nil)
	select {
	case s := <-h.out:
		t.Errorf("sample before the first tick: %+v", s)
	case <-time.After(100 * time.Millisecond):
	}
}

func TestRunStopsOnCancelAndReleasesHailo(t *testing.T) {
	closed := make(chan struct{})
	h := begin(t, func(c *Config) {
		c.Env.OpenHailo = func() (collector.HailoReader, error) { return closeSignal{closed}, nil }
	})
	h.f.Advance()
	want(t, h.tick(1), "temp.hailo", 54.3)
	h.stop()
	select {
	case err := <-h.done:
		if err != nil {
			t.Errorf("Run returned %v on cancel", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Run did not stop")
	}
	select {
	case <-closed:
	default:
		t.Error("the Hailo device was not released")
	}
}

type closeSignal struct{ c chan struct{} }

func (closeSignal) Temps() (collector.HailoTemps, error) {
	return collector.HailoTemps{TS0: 54.3, TS1: 53.9}, nil
}
func (c closeSignal) Close() error { close(c.c); return nil }

// DR-0003 9: the interval is at least 500 ms.
func TestIntervalBounds(t *testing.T) {
	f := testenv.New(t, fixtureDir)
	for _, d := range []time.Duration{0, -time.Second, 499 * time.Millisecond} {
		_, err := New(Config{Env: f.Env, Run: "r", Interval: d})
		if !errors.Is(err, exitcode.ErrUsage) {
			t.Errorf("interval %v: err = %v, want a usage error", d, err)
		}
	}
	if _, err := New(Config{Env: f.Env, Run: "r", Interval: 500 * time.Millisecond}); err != nil {
		t.Errorf("500ms: %v", err)
	}
}
