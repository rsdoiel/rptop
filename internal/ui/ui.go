// Package ui is the rptop dashboard (DR-0004): a Bubble Tea model that draws
// the samples of the sampler and drives the recorder. The model is a function
// of messages; the only I/O it causes is through commands that call the
// Recorder it was given, so tests feed it messages and read View().
package ui

import (
	"fmt"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/rsdoiel/rptop/internal/recorder"
	"github.com/rsdoiel/rptop/internal/sample"
)

// MinWidth and MinHeight are the smallest terminal the panels fit (DR-0004 3).
const (
	MinWidth  = 72
	MinHeight = 20
)

// historyLen is how many samples the sparklines remember (DR-0004 5).
const historyLen = 60

/** Recorder is what the dashboard needs of the recorder; *recorder.Controller
 * satisfies it.
 *
 * Example:
 *   m := ui.New(opts, controller)
 */
type Recorder interface {
	Start(label, path string) (recorder.Status, error)
	Stop(reason string) (recorder.Status, error)
	Marker(text string) error
	Status() recorder.Status
}

/** Options configures the dashboard.
 *
 * Fields:
 *   Host (string)                — the host name for the header
 *   Interval (time.Duration)     — the sampling interval, for the help text
 *   WarnTemp, CritTemp (float64) — the colour thresholds in °C
 *   Out (string)                 — where recordings started with r go ("" is the current directory)
 *   Uptime (func() time.Duration) — time since boot; nil hides it
 *
 * Example:
 *   ui.Options{Host: "harvey", WarnTemp: 60, CritTemp: 80}
 */
type Options struct {
	Host               string
	Interval           time.Duration
	WarnTemp, CritTemp float64
	Out                string
	Uptime             func() time.Duration
}

/** SampleMsg delivers one sample from the sampler.
 *
 * Example:
 *   program.Send(ui.SampleMsg{S: s})
 */
type SampleMsg struct{ S sample.Sample }

/** RecEndedMsg reports that the recorder ended a run by itself (low space or a
 * write error); the dashboard shows a banner and carries on.
 *
 * Fields:
 *   Status (recorder.Status) — the finished run
 *   Err (error)              — why it ended
 *
 * Example:
 *   program.Send(ui.RecEndedMsg{Status: ctl.Status(), Err: err})
 */
type RecEndedMsg struct {
	Status recorder.Status
	Err    error
}

type recStateMsg struct{ st recorder.Status }
type recErrMsg struct{ text string }

type mode int

const (
	modeNormal mode = iota
	modeLabel
	modeMarker
)

// view is what the screen shows; pausing freezes a copy of it.
type view struct {
	vals  map[string]*float64
	hist  map[string][]*float64
	cores []string
	ts    time.Time
	have  bool
}

/** Model is the dashboard. Create it with New and give it to tea.NewProgram.
 *
 * Example:
 *   p := tea.NewProgram(ui.New(opts, controller))
 */
type Model struct {
	opts   Options
	rec    Recorder
	w, h   int
	live   view
	frozen *view
	hist   map[string][]*float64
	paused bool
	help   bool
	mode   mode
	input  textinput.Model
	banner string
	status recorder.Status
	up     time.Duration
}

// histNames are the elements with a sparkline.
var histNames = []string{"temp.soc", "temp.pmic", "temp.hailo", "temp.rp1", "cpu.total"}

/** New returns the dashboard model.
 *
 * Parameters:
 *   o   (Options)  — thresholds, host, output directory
 *   rec (Recorder) — starts and stops recordings
 *
 * Returns:
 *   Model — ready for tea.NewProgram
 *
 * Example:
 *   m := ui.New(ui.Options{Host: "harvey", WarnTemp: 60, CritTemp: 80}, controller)
 */
func New(o Options, rec Recorder) Model {
	in := textinput.New()
	in.Prompt = ""
	in.CharLimit = 80
	return Model{opts: o, rec: rec, input: in, hist: map[string][]*float64{}}
}

// Init does nothing: the sampler sends SampleMsg values from outside.
func (m Model) Init() tea.Cmd { return nil }

// Update handles a message.
func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.w, m.h = msg.Width, msg.Height
	case SampleMsg:
		m.ingest(msg.S)
	case RecEndedMsg:
		m.status = msg.Status
		m.banner = fmt.Sprintf("recording ended (%s): %v", msg.Status.Reason, msg.Err)
	case recStateMsg:
		m.status = msg.st
	case recErrMsg:
		m.banner = msg.text
	case tea.KeyPressMsg:
		return m.key(msg)
	}
	return m, nil
}

func (m *Model) ingest(s sample.Sample) {
	v := view{vals: map[string]*float64{}, ts: s.TS, have: true}
	for _, e := range s.Entries {
		v.vals[e.Name] = e.Value
		if strings.HasPrefix(e.Name, "cpu.core") {
			v.cores = append(v.cores, e.Name)
		}
	}
	h := make(map[string][]*float64, len(m.hist))
	for k, old := range m.hist {
		h[k] = old
	}
	for _, n := range histNames {
		x := append(h[n][:len(h[n]):len(h[n])], v.vals[n])
		if len(x) > historyLen {
			x = x[len(x)-historyLen:]
		}
		h[n] = x
	}
	m.hist = h
	v.hist = h
	m.live = v
	if m.opts.Uptime != nil {
		m.up = m.opts.Uptime()
	}
	m.status = m.rec.Status()
}

func (m Model) shown() view {
	if m.paused && m.frozen != nil {
		return *m.frozen
	}
	return m.live
}

func (m Model) quit() tea.Cmd {
	rec := m.rec
	return func() tea.Msg {
		if rec.Status().Recording {
			rec.Stop(sample.ReasonStopped)
		}
		return tea.Quit()
	}
}

func (m Model) startCmd(label string) tea.Cmd {
	rec, out := m.rec, m.opts.Out
	return func() tea.Msg {
		st, err := rec.Start(label, out)
		if err != nil {
			return recErrMsg{"cannot start recording: " + err.Error()}
		}
		return recStateMsg{st}
	}
}

func (m Model) stopCmd() tea.Cmd {
	rec := m.rec
	return func() tea.Msg {
		st, err := rec.Stop(sample.ReasonStopped)
		if err != nil {
			return recErrMsg{"recording stopped with an error: " + err.Error()}
		}
		return recStateMsg{st}
	}
}

func (m Model) markerCmd(text string) tea.Cmd {
	rec := m.rec
	return func() tea.Msg {
		if err := rec.Marker(text); err != nil {
			return recErrMsg{"cannot write marker: " + err.Error()}
		}
		return nil
	}
}

func (m Model) key(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	s := msg.String()
	m.banner = ""
	if s == "ctrl+c" {
		return m, m.quit()
	}
	if m.mode != modeNormal {
		switch s {
		case "esc":
			m.mode = modeNormal
			m.input.Reset()
			m.input.Blur()
		case "enter":
			text := strings.TrimSpace(m.input.Value())
			was := m.mode
			m.mode = modeNormal
			m.input.Reset()
			m.input.Blur()
			switch {
			case was == modeLabel:
				return m, m.startCmd(text)
			case text != "":
				return m, m.markerCmd(text)
			}
		default:
			var cmd tea.Cmd
			m.input, cmd = m.input.Update(msg)
			return m, cmd
		}
		return m, nil
	}
	if m.help {
		switch s {
		case "esc", "?":
			m.help = false
		case "q":
			return m, m.quit()
		}
		return m, nil
	}
	switch s {
	case "q":
		return m, m.quit()
	case "r":
		if m.status.Recording {
			return m, m.stopCmd()
		}
		return m, m.startCmd("")
	case "R":
		if m.status.Recording {
			m.banner = "already recording; press r to stop first"
			return m, nil
		}
		m.mode = modeLabel
		return m, m.input.Focus()
	case "m":
		if !m.status.Recording {
			m.banner = "not recording: press r to start"
			return m, nil
		}
		m.mode = modeMarker
		return m, m.input.Focus()
	case "p":
		m.paused = !m.paused
		if m.paused {
			f := m.live
			f.hist = make(map[string][]*float64, len(m.live.hist))
			for k, v := range m.live.hist {
				f.hist[k] = append([]*float64(nil), v...)
			}
			m.frozen = &f
		} else {
			m.frozen = nil
		}
	case "?":
		m.help = true
	}
	return m, nil
}

// View draws the dashboard.
func (m Model) View() tea.View {
	var content string
	switch {
	case m.w < MinWidth || m.h < MinHeight:
		content = fmt.Sprintf("terminal too small\nneed %dx%d, have %dx%d", MinWidth, MinHeight, m.w, m.h)
	default:
		content = m.render()
	}
	v := tea.NewView(content)
	v.AltScreen = true
	return v
}

var (
	green  = lipgloss.NewStyle().Foreground(lipgloss.Color("2"))
	yellow = lipgloss.NewStyle().Foreground(lipgloss.Color("3"))
	red    = lipgloss.NewStyle().Foreground(lipgloss.Color("1"))
	dim    = lipgloss.NewStyle().Faint(true)
	bold   = lipgloss.NewStyle().Bold(true)
)

func paint(l Level, s string) string {
	switch l {
	case Green:
		return green.Render(s)
	case Yellow:
		return yellow.Render(s)
	case Red:
		return red.Render(s)
	}
	return dim.Render(s)
}

func pad(s string, w int) string {
	sw := ansi.StringWidth(s)
	if sw > w {
		return ansi.Truncate(s, w, "…")
	}
	return s + strings.Repeat(" ", w-sw)
}

func (m Model) render() string {
	W := m.w
	inner := W - 4
	row := func(s string) string { return "│ " + pad(s, inner) + " │" }
	sep := func(title string) string {
		t := "├ " + title + " "
		return t + strings.Repeat("─", max(W-1-ansi.StringWidth(t), 0)) + "┤"
	}
	v := m.shown()
	var out []string
	out = append(out, m.header(W))

	if m.help {
		for _, l := range []string{
			"q, ctrl+c   quit (an open recording is ended first)",
			"r           start a recording, or stop the open one",
			"R           ask for a label, then start a recording",
			"m           ask for marker text and write it to the recording",
			"p           pause the display (sampling and recording go on)",
			"?           show or hide this help",
			"esc         close a prompt or this help",
			"",
			"recordings are JSONL files; see rptop help record",
		} {
			out = append(out, row(l))
		}
		out = append(out, m.footer(W, "esc close"))
		return strings.Join(out, "\n")
	}
	if !v.have {
		out = append(out, row(dim.Render("waiting for the first sample…")))
		if m.banner != "" {
			out = append(out, row(red.Render("! "+m.banner)))
		}
		out = append(out, m.footer(W, ""))
		return strings.Join(out, "\n")
	}

	num := func(name string) (float64, bool) {
		if p := v.vals[name]; p != nil {
			return *p, true
		}
		return 0, false
	}
	na := dim.Render("n/a")
	pct := func(name string) string {
		if x, ok := num(name); ok {
			return fmt.Sprintf("%5.1f%%", x)
		}
		return fmt.Sprintf("%6s", na)
	}
	frac := func(name string) float64 { x, _ := num(name); return x / 100 }

	// CPU
	cpu := "CPU  " + bar(frac("cpu.total"), 16) + " " + pct("cpu.total")
	if x, ok := num("cpu.freq"); ok {
		cpu += fmt.Sprintf("  %.0f MHz", x)
	} else {
		cpu += "  " + na + " MHz"
	}
	if x, ok := num("cpu.load"); ok {
		cpu += fmt.Sprintf("  load %.2f", x)
	} else {
		cpu += "  load " + na
	}
	out = append(out, row(cpu))
	const cell = 16
	perRow := max((inner+1)/(cell+1), 1)
	for i := 0; i < len(v.cores); i += perRow {
		var cells []string
		for j := i; j < i+perRow && j < len(v.cores); j++ {
			n := v.cores[j]
			cells = append(cells, fmt.Sprintf("c%s %s %s", strings.TrimPrefix(n, "cpu.core"), bar(frac(n), 4), pct(n)))
		}
		out = append(out, row("  "+strings.Join(cells, " ")))
	}
	out = append(out, row("     "+dim.Render(Spark(v.hist["cpu.total"], 5, min(historyLen, inner-5)))))

	// Memory
	mem := "MEM  "
	used, uok := num("mem.used")
	total, tok := num("mem.total")
	switch {
	case uok && tok && total > 0:
		mem += bar(used/total, 16) + " " + Bytes(used) + " / " + Bytes(total)
	default:
		mem += bar(0, 16) + " " + na
	}
	if x, ok := num("mem.swap"); ok {
		mem += "  swap " + Bytes(x)
	} else {
		mem += "  swap " + na
	}
	out = append(out, row(mem))

	// Temperatures
	out = append(out, sep("Temperatures"))
	for _, t := range []struct{ label, name string }{{"SoC", "temp.soc"}, {"PMIC", "temp.pmic"}, {"Hailo", "temp.hailo"}, {"RP1", "temp.rp1"}} {
		val := fmt.Sprintf("%9s", na)
		if x, ok := num(t.name); ok {
			val = paint(TempLevel(x, m.opts.WarnTemp, m.opts.CritTemp), fmt.Sprintf("%6.1f °C", x))
		}
		sp := Spark(v.hist[t.name], 5, min(historyLen, inner-6-9-2))
		out = append(out, row(fmt.Sprintf("%-6s%s  %s", t.label, val, dim.Render(sp))))
	}

	// Power and health
	out = append(out, sep("Power / Health"))
	tn, tnok := num("throttle.now")
	ts, tsok := num("throttle.since_boot")
	if tnok && tsok {
		lvl := ThrottleLevel(tn, ts)
		out = append(out, row("Throttle  "+paint(lvl, "now: "+flags(tn))))
		out = append(out, row(paint(lvl, "since boot: "+flags(ts))))
	} else {
		out = append(out, row("Throttle  "+na))
	}
	fan, volts := na, na
	if x, ok := num("fan.speed"); ok {
		fan = fmt.Sprintf("%.0f rpm", x)
	}
	if x, ok := num("volts.core"); ok {
		volts = fmt.Sprintf("%.4f V", x)
	}
	out = append(out, row(fmt.Sprintf("Fan  %s     Core  %s", fan, volts)))

	// I/O
	out = append(out, sep("Disk / Net"))
	rate := func(name string) string {
		if x, ok := num(name); ok {
			return Rate(x)
		}
		return na
	}
	out = append(out, row("Disk  read "+rate("disk.read")+"   write "+rate("disk.write")))
	out = append(out, row("Net   rx "+rate("net.rx")+"   tx "+rate("net.tx")))

	if m.banner != "" {
		out = append(out, row(red.Render("! "+m.banner)))
	}
	out = append(out, m.footer(W, ""))
	return strings.Join(out, "\n")
}

// flags names the bits of a throttle word, or "none".
func flags(v float64) string {
	var names []string
	for i, n := range sample.ThrottleBits {
		if int(v)&(1<<i) != 0 {
			names = append(names, n)
		}
	}
	if len(names) == 0 {
		return "none"
	}
	return strings.Join(names, ", ")
}

func (m Model) header(W int) string {
	build := func(withUptime bool) string {
		segs := []string{bold.Render("rptop")}
		if m.opts.Host != "" {
			segs = append(segs, m.opts.Host)
		}
		if withUptime && m.opts.Uptime != nil && m.live.have {
			segs = append(segs, uptimeText(int(m.up.Seconds())))
		}
		if m.paused {
			segs = append(segs, yellow.Render("paused"))
		}
		if st := m.status; st.Recording {
			el := m.shownTime().Sub(st.Started)
			if el < 0 {
				el = 0
			}
			secs := int(el.Seconds())
			segs = append(segs, red.Render("rec ●")+" "+filepath.Base(st.Path)+" "+strconv.Itoa(st.Samples)+" samples "+fmt.Sprintf("%d:%02d", secs/60, secs%60))
		} else {
			segs = append(segs, dim.Render("rec ○ off"))
		}
		return "┌ " + strings.Join(segs, " ─ ") + " "
	}
	t := build(true)
	if ansi.StringWidth(t) > W-1 { // the uptime is the first thing to go
		t = build(false)
	}
	if ansi.StringWidth(t) > W-1 {
		t = ansi.Truncate(t, W-2, "…") + " "
	}
	return t + strings.Repeat("─", max(W-1-ansi.StringWidth(t), 0)) + "┐"
}

func (m Model) shownTime() time.Time { return m.live.ts }

func (m Model) footer(W int, override string) string {
	text := "q quit  r record  R label  m marker  p pause  ? help"
	switch {
	case m.mode == modeLabel:
		text = "label: " + m.input.View() + "   (enter starts, esc cancels)"
	case m.mode == modeMarker:
		text = "marker: " + m.input.View() + "   (enter writes, esc cancels)"
	case override != "":
		text = override
	}
	t := "└ " + text + " "
	if ansi.StringWidth(t) > W-1 {
		t = ansi.Truncate(t, W-2, "…") + " "
	}
	return t + strings.Repeat("─", max(W-1-ansi.StringWidth(t), 0)) + "┘"
}
