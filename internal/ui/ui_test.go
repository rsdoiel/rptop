package ui

import (
	"errors"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/rsdoiel/rptop/internal/recorder"
	"github.com/rsdoiel/rptop/internal/sample"
)

var update = flag.Bool("update", false, "rewrite the golden screens")

var t0 = time.Date(2026, 11, 15, 14, 30, 12, 0, time.UTC)

// fakeRec is the recorder the model talks to.
type fakeRec struct {
	st       recorder.Status
	starts   [][2]string
	stops    []string
	markers  []string
	startErr error
	markErr  error
}

func (f *fakeRec) Start(label, path string) (recorder.Status, error) {
	f.starts = append(f.starts, [2]string{label, path})
	if f.startErr != nil {
		return recorder.Status{}, f.startErr
	}
	f.st = recorder.Status{Recording: true, Path: "/runs/rptop-20261115-143012.jsonl", Run: "20261115-143012", Label: label, Started: t0}
	return f.st, nil
}
func (f *fakeRec) Stop(reason string) (recorder.Status, error) {
	f.stops = append(f.stops, reason)
	f.st.Recording, f.st.Reason = false, reason
	return f.st, nil
}
func (f *fakeRec) Marker(text string) error {
	if f.markErr != nil {
		return f.markErr
	}
	f.markers = append(f.markers, text)
	return nil
}
func (f *fakeRec) Status() recorder.Status { return f.st }

var names = []string{"temp.soc", "temp.pmic", "temp.hailo", "temp.rp1", "cpu.total", "cpu.core0", "cpu.core1", "cpu.core2", "cpu.core3",
	"cpu.freq", "cpu.load", "mem.used", "mem.total", "mem.swap", "fan.speed", "volts.core", "disk.read", "disk.write", "net.rx", "net.tx",
	"throttle.now", "throttle.since_boot", "self.cpu", "self.rss"}

var base = map[string]float64{
	"temp.soc": 54.1, "temp.pmic": 49.2, "temp.hailo": 61.5, "temp.rp1": 51.4, "cpu.total": 38.2,
	"cpu.core0": 31, "cpu.core1": 45.1, "cpu.core2": 22.4, "cpu.core3": 54.3, "cpu.freq": 2400, "cpu.load": 0.9,
	"mem.used": 2254857830, "mem.total": 17005985792, "mem.swap": 0, "fan.speed": 3725, "volts.core": 0.7838,
	"disk.read": 12582912, "disk.write": 3145728, "net.rx": 1258291, "net.tx": 104857,
	"throttle.now": 0, "throttle.since_boot": 14, "self.cpu": 0.6, "self.rss": 18874368,
}

// smp builds a sample; over replaces a value, and a NaN-free nil is made with null.
func smp(seq int, over map[string]float64, null ...string) sample.Sample {
	s := sample.Sample{Run: "r", Seq: seq, TS: t0.Add(time.Duration(seq+1) * time.Second), DtMS: 1000}
	isNull := map[string]bool{}
	for _, n := range null {
		isNull[n] = true
	}
	for _, n := range names {
		e := sample.Entry{Name: n}
		if !isNull[n] {
			v := base[n]
			if o, ok := over[n]; ok {
				v = o
			}
			e.Value = sample.Num(v)
		}
		s.Entries = append(s.Entries, e)
	}
	return s
}

func newModel(t *testing.T, w, h int) (Model, *fakeRec) {
	t.Helper()
	rec := &fakeRec{}
	m := New(Options{Host: "harvey", Interval: time.Second, WarnTemp: 60, CritTemp: 80, Out: "/runs",
		Uptime: func() time.Duration { return 34*time.Hour + 17*time.Minute }}, rec)
	return send(m, tea.WindowSizeMsg{Width: w, Height: h}), rec
}

func send(m Model, msg tea.Msg) Model {
	next, _ := m.Update(msg)
	return next.(Model)
}

// sendCmd also runs the returned command and feeds its message back in.
func sendCmd(t *testing.T, m Model, msg tea.Msg) (Model, tea.Msg) {
	t.Helper()
	next, cmd := m.Update(msg)
	m = next.(Model)
	if cmd == nil {
		return m, nil
	}
	out := cmd()
	if out == nil {
		return m, nil
	}
	if _, quit := out.(tea.QuitMsg); quit {
		return m, out
	}
	return send(m, out), out
}

func key(s string) tea.KeyPressMsg {
	switch s {
	case "enter":
		return tea.KeyPressMsg{Code: tea.KeyEnter}
	case "esc":
		return tea.KeyPressMsg{Code: tea.KeyEscape}
	case "ctrl+c":
		return tea.KeyPressMsg{Code: 'c', Mod: tea.ModCtrl}
	}
	r := []rune(s)[0]
	return tea.KeyPressMsg{Code: r, Text: s}
}

func typeText(m Model, s string) Model {
	for _, r := range s {
		m = send(m, key(string(r)))
	}
	return m
}

func plain(m Model) string { return ansi.Strip(m.View().Content) }

func lineCount(s string) int { return len(strings.Split(s, "\n")) }

func TestTempLevelBoundaries(t *testing.T) {
	cases := []struct {
		v    float64
		want Level
	}{{40, Green}, {59.99, Green}, {60, Yellow}, {79.99, Yellow}, {80, Red}, {95, Red}}
	for _, c := range cases {
		if got := TempLevel(c.v, 60, 80); got != c.want {
			t.Errorf("TempLevel(%v) = %v, want %v", c.v, got, c.want)
		}
	}
	if got := TempLevel(70, 65, 70); got != Red {
		t.Errorf("custom thresholds: TempLevel(70, 65, 70) = %v, want red", got)
	}
}

func TestThrottleLevel(t *testing.T) {
	cases := []struct {
		now, since float64
		want       Level
	}{{0, 0, Green}, {0, 14, Yellow}, {1, 0, Red}, {4, 14, Red}}
	for _, c := range cases {
		if got := ThrottleLevel(c.now, c.since); got != c.want {
			t.Errorf("ThrottleLevel(%v, %v) = %v, want %v", c.now, c.since, got, c.want)
		}
	}
}

func fl(vs ...float64) []*float64 {
	var out []*float64
	for _, v := range vs {
		v := v
		out = append(out, &v)
	}
	return out
}

func TestSparkline(t *testing.T) {
	// A rising run uses the whole scale.
	if got := Spark(fl(50, 51, 52, 53, 54, 55, 56, 57), 5, 60); got != "▁▂▃▄▅▆▇█" && got != "▁▂▃▄▅▆▇▇" && len([]rune(got)) != 8 {
		t.Errorf("rising = %q", got)
	}
	got := []rune(Spark(fl(50, 55), 5, 60))
	if got[0] != '▁' || got[1] != '█' {
		t.Errorf("0..span = %q", string(got))
	}
	// Steady value: a flat line, however small the wobble (floor of 5 on the scale).
	flat := Spark(fl(54.0, 54.1, 54.0, 54.1, 54.0), 5, 60)
	for _, r := range flat {
		if r != '▁' && r != '▂' {
			t.Errorf("a 0.1 wobble drew %q, want a flat line", flat)
			break
		}
	}
	if s := Spark(fl(54, 54, 54), 5, 60); s != "▁▁▁" {
		t.Errorf("steady = %q, want ▁▁▁", s)
	}
	// A null sample is a gap.
	g := []*float64{ptr(50), nil, ptr(55)}
	if s := Spark(g, 5, 60); s != "▁ █" {
		t.Errorf("gap = %q, want %q", s, "▁ █")
	}
	// Only the last `width` samples are drawn.
	long := make([]*float64, 100)
	for i := range long {
		long[i] = ptr(float64(i))
	}
	if n := len([]rune(Spark(long, 5, 60))); n != 60 {
		t.Errorf("long history drew %d characters, want 60", n)
	}
	if n := len([]rune(Spark(long, 5, 30))); n != 30 {
		t.Errorf("width 30 drew %d characters", n)
	}
	if Spark(nil, 5, 60) != "" {
		t.Error("empty history drew something")
	}
}

func ptr(v float64) *float64 { return &v }

func TestHumanUnits(t *testing.T) {
	for in, want := range map[float64]string{0: "0 B/s", 512: "512 B/s", 1536: "1.5 KB/s", 1048576: "1.0 MB/s", 12582912: "12.0 MB/s", 3.5 * 1073741824: "3.5 GB/s"} {
		if got := Rate(in); got != want {
			t.Errorf("Rate(%v) = %q, want %q", in, got, want)
		}
	}
	for in, want := range map[float64]string{0: "0 B", 2254857830: "2.1G", 17005985792: "15.8G", 524288: "512K"} {
		if got := Bytes(in); got != want {
			t.Errorf("Bytes(%v) = %q, want %q", in, got, want)
		}
	}
}

func TestViewFillsTheTerminalExactly(t *testing.T) {
	for _, sz := range [][2]int{{72, 20}, {80, 24}, {100, 30}} {
		m, _ := newModel(t, sz[0], sz[1])
		m = send(m, SampleMsg{smp(0, nil)})
		lines := strings.Split(plain(m), "\n")
		if len(lines) > sz[1] {
			t.Errorf("%v: %d lines, more than the %d rows", sz, len(lines), sz[1])
		}
		for i, l := range lines {
			if w := ansi.StringWidth(l); w != sz[0] {
				t.Errorf("%v: line %d is %d columns, want %d: %q", sz, i, w, sz[0], l)
			}
		}
	}
}

func TestAltScreen(t *testing.T) {
	m, _ := newModel(t, 80, 24)
	if !m.View().AltScreen {
		t.Error("the dashboard must use the alternate screen")
	}
}

func TestTooSmall(t *testing.T) {
	for _, sz := range [][2]int{{71, 20}, {72, 19}, {40, 10}} {
		m, _ := newModel(t, sz[0], sz[1])
		m = send(m, SampleMsg{smp(0, nil)})
		v := plain(m)
		if !strings.Contains(v, "terminal too small") || !strings.Contains(v, "72x20") || strings.Contains(v, "SoC") {
			t.Errorf("%v: %q", sz, v)
		}
		if lineCount(v) > sz[1] {
			t.Errorf("%v: message is %d lines", sz, lineCount(v))
		}
	}
	m, _ := newModel(t, 60, 10)
	m = send(m, SampleMsg{smp(0, nil)})
	m = send(m, tea.WindowSizeMsg{Width: 90, Height: 30}) // recovers when resized
	if v := plain(m); !strings.Contains(v, "SoC") || strings.Contains(v, "too small") {
		t.Errorf("after resize: %q", v)
	}
}

func TestPanelsCarryTheReadings(t *testing.T) {
	m, _ := newModel(t, 80, 24)
	m = send(m, SampleMsg{smp(0, nil)})
	v := plain(m)
	for _, want := range []string{
		"rptop", "harvey", "up 1d 10h", "rec ○ off",
		"CPU", "38.2%", "2400 MHz", "load 0.90", "c0", "c3", "54.3%",
		"MEM", "2.1G", "15.8G", "swap 0 B",
		"SoC", "54.1 °C", "PMIC", "49.2 °C", "Hailo", "61.5 °C", "RP1", "51.4 °C",
		"now: none", "since boot: arm frequency capped, throttled, soft temperature limit",
		"3725 rpm", "0.7838 V",
		"12.0 MB/s", "3.0 MB/s", "1.2 MB/s", "102.4 KB/s", "q quit",
	} {
		if !strings.Contains(v, want) {
			t.Errorf("view lacks %q:\n%s", want, v)
		}
	}
}

// DR-0004 4: a reading that is null shows n/a, never a zero.
func TestNullShowsNA(t *testing.T) {
	m, _ := newModel(t, 80, 24)
	m = send(m, SampleMsg{smp(0, nil, "temp.hailo", "fan.speed", "net.rx")})
	v := plain(m)
	var hailo, fan, net string
	for _, l := range strings.Split(v, "\n") {
		switch {
		case strings.Contains(l, "Hailo"):
			hailo = l
		case strings.Contains(l, "Fan"):
			fan = l
		case strings.Contains(l, "Net"):
			net = l
		}
	}
	for name, l := range map[string]string{"hailo": hailo, "fan": fan, "net": net} {
		if !strings.Contains(l, "n/a") {
			t.Errorf("%s line has no n/a: %q", name, l)
		}
	}
	if strings.Contains(hailo, "0.0") {
		t.Errorf("hailo line shows a zero: %q", hailo)
	}
}

func TestNoSampleYet(t *testing.T) {
	m, _ := newModel(t, 80, 24)
	v := plain(m)
	if !strings.Contains(v, "waiting for the first sample") {
		t.Errorf("view before any sample: %q", v)
	}
}

func TestSparklinesFollowTheSamples(t *testing.T) {
	m, _ := newModel(t, 80, 24)
	for i := 0; i < 10; i++ {
		m = send(m, SampleMsg{smp(i, map[string]float64{"temp.soc": 50 + float64(i)})})
	}
	var soc string
	for _, l := range strings.Split(plain(m), "\n") {
		if strings.Contains(l, "SoC") {
			soc = l
		}
	}
	if !strings.Contains(soc, "▁") || !strings.Contains(soc, "█") {
		t.Errorf("SoC line has no rising sparkline: %q", soc)
	}
	if !strings.Contains(soc, "59.0 °C") {
		t.Errorf("SoC line shows the latest value? %q", soc)
	}
}

func TestHistoryIsBounded(t *testing.T) {
	m, _ := newModel(t, 80, 24)
	for i := 0; i < 200; i++ {
		m = send(m, SampleMsg{smp(i, nil)})
	}
	if n := len(m.hist["temp.soc"]); n != 60 {
		t.Errorf("history holds %d samples, want 60", n)
	}
}

func TestPauseFreezesTheDisplayOnly(t *testing.T) {
	m, _ := newModel(t, 80, 24)
	m = send(m, SampleMsg{smp(0, nil)})
	before := plain(m)
	m = send(m, key("p"))
	m = send(m, SampleMsg{smp(1, map[string]float64{"temp.soc": 70.0})})
	paused := plain(m)
	if !strings.Contains(paused, "paused") {
		t.Errorf("no paused marker: %q", paused)
	}
	if strings.Contains(paused, "70.0 °C") {
		t.Errorf("the display moved while paused")
	}
	if strings.ReplaceAll(strings.ReplaceAll(paused, "paused", ""), " ", "") == "" || !strings.Contains(paused, "54.1 °C") {
		t.Errorf("paused display lost its readings")
	}
	_ = before
	m = send(m, key("p"))
	if v := plain(m); strings.Contains(v, "paused") || !strings.Contains(v, "70.0 °C") {
		t.Errorf("resuming did not show the newest sample: %q", v)
	}
}

func TestColours(t *testing.T) {
	m, _ := newModel(t, 80, 24)
	m = send(m, SampleMsg{smp(0, map[string]float64{"temp.soc": 50, "temp.pmic": 65, "temp.hailo": 85})})
	raw := m.View().Content
	has := func(code, text string) bool { // the text sits inside a style that uses colour `code`
		i := strings.Index(raw, text)
		if i < 0 {
			return false
		}
		return strings.Contains(raw[max(0, i-40):i], "\x1b[") && strings.Contains(raw[max(0, i-40):i], code)
	}
	if !has("32m", "50.0 °C") {
		t.Errorf("a cool SoC is not green")
	}
	if !has("33m", "65.0 °C") {
		t.Errorf("a warm PMIC is not yellow")
	}
	if !has("31m", "85.0 °C") {
		t.Errorf("a hot Hailo is not red")
	}
}

func TestQuitStopsAnOpenRecordingFirst(t *testing.T) {
	for _, k := range []string{"q", "ctrl+c"} {
		m, rec := newModel(t, 80, 24)
		m, _ = sendCmd(t, m, key("r"))
		if !rec.st.Recording {
			t.Fatal("r did not start a recording")
		}
		_, out := sendCmd(t, m, key(k))
		if _, ok := out.(tea.QuitMsg); !ok {
			t.Errorf("%s: message = %T, want a quit", k, out)
		}
		if len(rec.stops) != 1 || rec.stops[0] != sample.ReasonStopped {
			t.Errorf("%s: stops = %v, want [stopped]", k, rec.stops)
		}
	}
	m, rec := newModel(t, 80, 24)
	_, out := sendCmd(t, m, key("q"))
	if _, ok := out.(tea.QuitMsg); !ok || len(rec.stops) != 0 {
		t.Errorf("idle quit: %T, stops %v", out, rec.stops)
	}
}

func TestRecordKey(t *testing.T) {
	m, rec := newModel(t, 80, 24)
	m = send(m, SampleMsg{smp(0, nil)})
	m, _ = sendCmd(t, m, key("r"))
	if len(rec.starts) != 1 || rec.starts[0] != [2]string{"", "/runs"} {
		t.Errorf("starts = %v, want one start with no label in /runs", rec.starts)
	}
	m = send(m, SampleMsg{smp(1, nil)})
	if v := plain(m); !strings.Contains(v, "rec ●") || !strings.Contains(v, "rptop-20261115-143012.jsonl") {
		t.Errorf("header does not show the recording: %q", strings.Split(v, "\n")[0])
	}
	m, _ = sendCmd(t, m, key("r"))
	if len(rec.stops) != 1 || rec.stops[0] != sample.ReasonStopped {
		t.Errorf("stops = %v", rec.stops)
	}
	if v := plain(m); !strings.Contains(v, "rec ○ off") {
		t.Errorf("header after stop: %q", strings.Split(v, "\n")[0])
	}
}

func TestHeaderCountsSamplesAndTime(t *testing.T) {
	m, rec := newModel(t, 80, 24)
	m, _ = sendCmd(t, m, key("r"))
	rec.st.Samples = 7
	m = send(m, SampleMsg{smp(64, nil)}) // 65 s after the recording started
	if h := strings.Split(plain(m), "\n")[0]; !strings.Contains(h, "7 samples") || !strings.Contains(h, "1:05") {
		t.Errorf("header = %q", h)
	}
}

func TestLabelPrompt(t *testing.T) {
	m, rec := newModel(t, 80, 24)
	m = send(m, key("R"))
	if v := plain(m); !strings.Contains(v, "label:") {
		t.Errorf("no label prompt: %q", v)
	}
	m = typeText(m, "before ollama")
	m, _ = sendCmd(t, m, key("enter"))
	if len(rec.starts) != 1 || rec.starts[0][0] != "before ollama" {
		t.Errorf("starts = %v", rec.starts)
	}
	if strings.Contains(plain(m), "label:") {
		t.Error("prompt still open after Enter")
	}
	// Keys typed in the prompt are text, not commands.
	m, rec = newModel(t, 80, 24)
	m = send(m, key("R"))
	m = typeText(m, "qrpm?")
	if len(rec.starts)+len(rec.stops) != 0 || strings.Contains(plain(m), "help") {
		t.Error("letters typed in the prompt acted as commands")
	}
	// Esc cancels.
	m = send(m, key("esc"))
	if strings.Contains(plain(m), "label:") || len(rec.starts) != 0 {
		t.Error("Esc did not cancel the prompt")
	}
}

func TestLabelWhileRecordingIsABanner(t *testing.T) {
	m, rec := newModel(t, 80, 24)
	m, _ = sendCmd(t, m, key("r"))
	m = send(m, key("R"))
	if v := plain(m); !strings.Contains(v, "already recording") || strings.Contains(v, "label:") {
		t.Errorf("R while recording: %q", v)
	}
	if len(rec.starts) != 1 {
		t.Errorf("starts = %v", rec.starts)
	}
}

func TestMarkerPrompt(t *testing.T) {
	m, rec := newModel(t, 80, 24)
	m = send(m, key("m"))
	if v := plain(m); !strings.Contains(v, "not recording") {
		t.Errorf("m while idle: %q", v)
	}
	m, _ = sendCmd(t, m, key("r"))
	m = send(m, key("m"))
	if v := plain(m); !strings.Contains(v, "marker:") {
		t.Errorf("no marker prompt: %q", v)
	}
	m = typeText(m, "start load")
	m, _ = sendCmd(t, m, key("enter"))
	if len(rec.markers) != 1 || rec.markers[0] != "start load" {
		t.Errorf("markers = %v", rec.markers)
	}
	// An empty marker is cancelled, not written.
	m = send(m, key("m"))
	m, _ = sendCmd(t, m, key("enter"))
	if len(rec.markers) != 1 {
		t.Errorf("an empty marker was written: %v", rec.markers)
	}
}

func TestHelpToggle(t *testing.T) {
	m, _ := newModel(t, 80, 24)
	m = send(m, SampleMsg{smp(0, nil)})
	m = send(m, key("?"))
	v := plain(m)
	for _, want := range []string{"quit", "record", "marker", "pause", "label"} {
		if !strings.Contains(v, want) {
			t.Errorf("help lacks %q", want)
		}
	}
	if strings.Contains(v, "SoC") {
		t.Error("help should replace the panels")
	}
	m = send(m, key("esc"))
	if !strings.Contains(plain(m), "SoC") {
		t.Error("esc did not close the help")
	}
	m = send(m, key("?"))
	m = send(m, key("?"))
	if strings.Contains(plain(m), "pause") && !strings.Contains(plain(m), "SoC") {
		t.Error("? did not toggle the help off")
	}
}

func TestBannersFromFailuresAndClearOnAKey(t *testing.T) {
	m, rec := newModel(t, 80, 24)
	rec.startErr = errors.New("/runs/x.jsonl: file exists: cannot create")
	m, _ = sendCmd(t, m, key("r"))
	if v := plain(m); !strings.Contains(v, "cannot start recording") || !strings.Contains(v, "file exists") {
		t.Errorf("start failure: %q", v)
	}
	m = send(m, key("?")) // any key clears it...
	m = send(m, key("esc"))
	if strings.Contains(plain(m), "cannot start") {
		t.Error("the banner outlived a key press")
	}
	// A run the recorder ended itself.
	m = send(m, RecEndedMsg{Status: recorder.Status{Reason: sample.ReasonLowSpace}, Err: errors.New("free space is below the guard")})
	if v := plain(m); !strings.Contains(v, "recording ended") || !strings.Contains(v, "low_space") {
		t.Errorf("RecEndedMsg: %q", v)
	}
	m = send(m, SampleMsg{smp(0, nil)}) // a sample alone does not clear it
	if !strings.Contains(plain(m), "recording ended") {
		t.Error("a sample cleared the banner")
	}
	m = send(m, key("x"))
	if strings.Contains(plain(m), "recording ended") {
		t.Error("a key press did not clear the banner")
	}
	// A marker that fails.
	m, rec = newModel(t, 80, 24)
	m, _ = sendCmd(t, m, key("r"))
	rec.markErr = errors.New("disk full")
	m = send(m, key("m"))
	m = typeText(m, "x")
	m, _ = sendCmd(t, m, key("enter"))
	if !strings.Contains(plain(m), "cannot write marker") {
		t.Errorf("marker failure: %q", plain(m))
	}
}

func TestLongBannerIsTruncatedToTheWidth(t *testing.T) {
	m, rec := newModel(t, 72, 24)
	rec.startErr = errors.New(strings.Repeat("very long error ", 20))
	m, _ = sendCmd(t, m, key("r"))
	for i, l := range strings.Split(plain(m), "\n") {
		if w := ansi.StringWidth(l); w != 72 {
			t.Errorf("line %d is %d wide: %q", i, w, l)
		}
	}
}

func TestManyCoresWrap(t *testing.T) {
	m, _ := newModel(t, 80, 30)
	s := smp(0, nil)
	for i := 4; i < 8; i++ {
		s.Entries = append(s.Entries, sample.Entry{Name: fmt.Sprintf("cpu.core%d", i), Value: sample.Num(10)})
	}
	m = send(m, SampleMsg{s})
	v := plain(m)
	if !strings.Contains(v, "c7") {
		t.Errorf("core 7 missing: %q", v)
	}
	for i, l := range strings.Split(v, "\n") {
		if w := ansi.StringWidth(l); w != 80 {
			t.Errorf("line %d is %d wide", i, w)
		}
	}
}

var ansiRE = regexp.MustCompile("\x1b\\[[0-9;]*m")

func TestGoldenScreen(t *testing.T) {
	m, rec := newModel(t, 80, 24)
	m, _ = sendCmd(t, m, key("r"))
	rec.st.Samples = 12
	for i := 0; i < 20; i++ {
		m = send(m, SampleMsg{smp(i, map[string]float64{
			"temp.soc": 50 + float64(i%10), "temp.pmic": 49.2, "temp.hailo": 55 + float64(i)*0.7, "cpu.total": 20 + float64(i*3%40)})})
	}
	got := plain(m) + "\n"
	golden := filepath.Join("testdata", "dashboard-80x24.txt")
	if *update {
		os.WriteFile(golden, []byte(got), 0o644)
	}
	want, err := os.ReadFile(golden)
	if err != nil {
		t.Fatalf("no golden screen (run with -update): %v", err)
	}
	if got != string(want) {
		t.Errorf("screen differs from %s:\n--- got\n%s--- want\n%s", golden, got, want)
	}
	_ = ansiRE
}
