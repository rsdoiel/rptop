// Package element is the registry of everything rptop can read: one Elem per
// element name, shared by the reading verbs, the sampler and the dashboard so
// all three report the same names, units and values.
package element

import (
	"errors"
	"fmt"
	"strconv"
	"sync"
	"time"

	"github.com/rsdoiel/rptop/internal/collector"
)

/** Env is the machine rptop reads: where the sensors are and how to open the
 * Hailo device. Tests inject fixtures; DefaultEnv is the real Pi.
 *
 * Fields:
 *   Sources   (collector.Sources)                      — sensor paths and the vcgencmd runner
 *   OpenHailo (func() (collector.HailoReader, error))  — opens the Hailo device; nil means absent
 *
 * Example:
 *   env := element.DefaultEnv()
 */
type Env struct {
	Sources   collector.Sources
	OpenHailo func() (collector.HailoReader, error)
}

/** DefaultEnv returns the Env for the running Pi.
 *
 * Returns:
 *   Env — real /sys and /proc sources and collector.OpenHailo
 *
 * Example:
 *   c := element.NewCtx(element.DefaultEnv())
 */
func DefaultEnv() Env {
	return Env{Sources: collector.DefaultSources(), OpenHailo: collector.OpenHailo}
}

/** Verbs lists the reading verbs in the order `rptop list` reports them.
 *
 * Example:
 *   for _, v := range element.Verbs { ... }
 */
var Verbs = []string{"temp", "cpu", "mem", "fan", "volts", "disk", "net"}

/** RateVerb reports whether a verb's readings are the difference between two
 * snapshots (cpu, disk, net) and so take an interval.
 *
 * Parameters:
 *   verb (string) — a verb name
 *
 * Returns:
 *   bool — true for cpu, disk and net
 *
 * Example:
 *   element.RateVerb("disk") // true
 */
func RateVerb(verb string) bool { return verb == "cpu" || verb == "disk" || verb == "net" }

/** Elem is one readable element.
 *
 * Fields:
 *   Verb, Short (string) — the group and the short name: "temp", "soc"
 *   Name (string)        — the dotted name: "temp.soc"
 *   Unit (string)        — "°C", "%", "bytes", "B/s", ... or "" for a bare number
 *   Digits (int)         — decimal places for the text forms
 *   Rate (bool)          — read needs a Prev and Cur snapshot (Ctx.SetRates)
 *   MinPeriod (time.Duration) — least time between reads when polled repeatedly; 0 means every tick
 *   Read  (func(*Ctx) (float64, error)) — the reading
 *   Probe (func(*Ctx) error) — availability without waiting; nil means Read
 *
 * Example:
 *   v, err := e.Read(ctx)
 */
type Elem struct {
	Verb, Short, Name string
	Unit              string
	Digits            int
	Rate              bool
	MinPeriod         time.Duration
	Read              func(*Ctx) (float64, error)
	Probe             func(*Ctx) error
}

// CPUSample is one core's counters in a snapshot, or the error reading them.
type CPUSample struct {
	T   collector.CPUTimes
	Err error
}

/** Snapshot is one reading of every cumulative counter the rates need.
 *
 * Fields:
 *   CPU   (map[int]CPUSample) — key -1 is all cores, 0..N-1 each core
 *   DiskR, DiskW (float64)    — bytes read and written, cumulative
 *   DiskErr (error)           — why the disk counters could not be read
 *   NetRx, NetTx (float64)    — bytes received and sent, cumulative
 *   NetErr (error)            — why the network counters could not be read
 *
 * Example:
 *   prev := element.Take(env.Sources)
 */
type Snapshot struct {
	CPU          map[int]CPUSample
	DiskR, DiskW float64
	DiskErr      error
	NetRx, NetTx float64
	NetErr       error
}

/** Take reads every cumulative counter once.
 *
 * Parameters:
 *   src (collector.Sources) — where to read
 *
 * Returns:
 *   Snapshot — the counters, each with its own error
 *
 * Example:
 *   s := element.Take(collector.DefaultSources())
 */
func Take(src collector.Sources) Snapshot {
	s := Snapshot{CPU: map[int]CPUSample{}}
	keys := []int{-1}
	if n, err := collector.CPUCores(src); err == nil {
		for i := 0; i < n; i++ {
			keys = append(keys, i)
		}
	} else {
		s.CPU[-1] = CPUSample{Err: err}
		keys = nil
	}
	for _, k := range keys {
		t, err := collector.CPUTimesOf(src, k)
		s.CPU[k] = CPUSample{T: t, Err: err}
	}
	s.DiskR, s.DiskW, s.DiskErr = collector.DiskBytes(src)
	s.NetRx, s.NetTx, s.NetErr = collector.NetBytes(src)
	return s
}

/** Ctx is the state elements read from: the environment, the two snapshots a
 * rate needs, and the Hailo device, opened the first time it is wanted and kept
 * until Close.
 *
 * Example:
 *   c := element.NewCtx(env)
 *   defer c.Close()
 */
type Ctx struct {
	Env Env

	prev, cur *Snapshot
	dt        time.Duration

	h *hailoState // shared by every copy made with WithRates
}

// hailoState is the one Hailo handle of a command, opened on first use.
type hailoState struct {
	mu    sync.Mutex
	r     collector.HailoReader
	err   error
	tried bool
}

/** NewCtx returns an empty Ctx for env.
 *
 * Parameters:
 *   env (Env) — the machine
 *
 * Returns:
 *   *Ctx — call Close when done
 *
 * Example:
 *   c := element.NewCtx(element.DefaultEnv())
 */
func NewCtx(env Env) *Ctx { return &Ctx{Env: env, h: &hailoState{}} }

/** SetRates gives the rate elements their two snapshots and the time between.
 *
 * Parameters:
 *   prev, cur (Snapshot)    — the earlier and later counters
 *   dt (time.Duration)      — the time between them
 *
 * Example:
 *   c.SetRates(a, b, time.Second)
 */
func (c *Ctx) SetRates(prev, cur Snapshot, dt time.Duration) {
	c.prev, c.cur, c.dt = &prev, &cur, dt
}

/** WithRates returns a copy of c carrying the two snapshots. The copy shares
 * c's Hailo handle, and nothing it holds changes afterwards, so a read still
 * running when the next tick starts cannot see the next tick's snapshots.
 *
 * Parameters:
 *   prev, cur (Snapshot) — the earlier and later counters
 *   dt (time.Duration)   — the time between them
 *
 * Returns:
 *   *Ctx — a copy; Close it through the original
 *
 * Example:
 *   tick := c.WithRates(a, b, time.Second)
 */
func (c *Ctx) WithRates(prev, cur Snapshot, dt time.Duration) *Ctx {
	n := *c
	n.prev, n.cur, n.dt = &prev, &cur, dt
	return &n
}

/** Close releases the Hailo device if it was opened.
 *
 * Example:
 *   defer c.Close()
 */
func (c *Ctx) Close() {
	c.h.mu.Lock()
	defer c.h.mu.Unlock()
	if c.h.r != nil {
		c.h.r.Close()
		c.h.r = nil
		c.h.err = fmt.Errorf("hailo: closed: %w", collector.ErrUnreachable)
	}
}

// ErrNoRates is a bug: a rate element was read without SetRates.
var ErrNoRates = errors.New("rate read without two snapshots")

func (c *Ctx) hailoTemp() (float64, error) {
	c.h.mu.Lock()
	if !c.h.tried {
		c.h.tried = true
		if c.Env.OpenHailo == nil {
			c.h.err = fmt.Errorf("hailo: %w", collector.ErrAbsent)
		} else {
			c.h.r, c.h.err = c.Env.OpenHailo()
		}
	}
	r, err := c.h.r, c.h.err
	c.h.mu.Unlock()
	if err != nil {
		return 0, err
	}
	return collector.TempHailo(r)
}

func (c *Ctx) cpuPercent(core int) (float64, error) {
	if c.prev == nil {
		return 0, ErrNoRates
	}
	a, ok := c.prev.CPU[core]
	if !ok {
		return 0, fmt.Errorf("no such core: %w", collector.ErrAbsent)
	}
	b := c.cur.CPU[core]
	if a.Err != nil {
		return 0, a.Err
	}
	if b.Err != nil {
		return 0, b.Err
	}
	return collector.CPUPercent(a.T, b.T)
}

func (c *Ctx) rate(pick func(*Snapshot) (float64, error)) (float64, error) {
	if c.prev == nil {
		return 0, ErrNoRates
	}
	a, err := pick(c.prev)
	if err != nil {
		return 0, err
	}
	b, err := pick(c.cur)
	if err != nil {
		return 0, err
	}
	return collector.Rate(a, b, c.dt)
}

func plain(f func(collector.Sources) (float64, error)) func(*Ctx) (float64, error) {
	return func(c *Ctx) (float64, error) { return f(c.Env.Sources) }
}

func mk(verb, short, unit string, digits int, read func(*Ctx) (float64, error)) Elem {
	return Elem{Verb: verb, Short: short, Name: verb + "." + short, Unit: unit, Digits: digits, Read: read}
}

/** CoreElem is the element for core N of the cpu verb.
 *
 * Parameters:
 *   n (int) — the core number
 *
 * Returns:
 *   Elem — named cpu.coreN, in percent
 *
 * Example:
 *   e := element.CoreElem(2) // cpu.core2
 */
func CoreElem(n int) Elem {
	e := mk("cpu", "core"+strconv.Itoa(n), "%", 1, func(c *Ctx) (float64, error) { return c.cpuPercent(n) })
	e.Rate = true
	e.Probe = func(c *Ctx) error { _, err := collector.CPUTimesOf(c.Env.Sources, n); return err }
	return e
}

/** Group returns a verb's elements in report order.
 *
 * Parameters:
 *   verb (string) — one of Verbs
 *   src (collector.Sources) — used to count the cores for cpu
 *
 * Returns:
 *   []Elem — nil for an unknown verb
 *
 * Example:
 *   for _, e := range element.Group("temp", src) { ... }
 */
func Group(verb string, src collector.Sources) []Elem {
	const slow = 5 * time.Second // vcgencmd readings that change slowly are polled less often
	switch verb {
	case "temp":
		pmic := mk(verb, "pmic", "°C", 1, plain(collector.TempPMIC))
		pmic.MinPeriod = slow
		return []Elem{
			mk(verb, "soc", "°C", 1, plain(collector.TempSoC)),
			pmic,
			mk(verb, "hailo", "°C", 1, func(c *Ctx) (float64, error) { return c.hailoTemp() }),
			mk(verb, "rp1", "°C", 1, plain(collector.TempRP1)),
		}
	case "cpu":
		total := mk(verb, "total", "%", 1, func(c *Ctx) (float64, error) { return c.cpuPercent(-1) })
		total.Rate = true
		total.Probe = func(c *Ctx) error { _, err := collector.CPUTimesOf(c.Env.Sources, -1); return err }
		out := []Elem{total}
		if n, err := collector.CPUCores(src); err == nil {
			for i := 0; i < n; i++ {
				out = append(out, CoreElem(i))
			}
		}
		return append(out,
			mk(verb, "freq", "MHz", 0, plain(collector.CPUFreqMHz)),
			mk(verb, "load", "", 2, plain(collector.Load1)))
	case "mem":
		return []Elem{
			mk(verb, "used", "bytes", 0, plain(collector.MemUsed)),
			mk(verb, "total", "bytes", 0, plain(collector.MemTotal)),
			mk(verb, "swap", "bytes", 0, plain(collector.SwapUsed)),
		}
	case "fan":
		return []Elem{mk(verb, "speed", "rpm", 0, plain(collector.FanRPM))}
	case "volts":
		core := mk(verb, "core", "V", 4, plain(collector.VoltsCore))
		core.MinPeriod = slow
		return []Elem{core}
	case "disk":
		probe := func(c *Ctx) error { _, _, err := collector.DiskBytes(c.Env.Sources); return err }
		r := mk(verb, "read", "B/s", 0, func(c *Ctx) (float64, error) {
			return c.rate(func(s *Snapshot) (float64, error) { return s.DiskR, s.DiskErr })
		})
		w := mk(verb, "write", "B/s", 0, func(c *Ctx) (float64, error) {
			return c.rate(func(s *Snapshot) (float64, error) { return s.DiskW, s.DiskErr })
		})
		r.Rate, w.Rate, r.Probe, w.Probe = true, true, probe, probe
		return []Elem{r, w}
	case "net":
		probe := func(c *Ctx) error { _, _, err := collector.NetBytes(c.Env.Sources); return err }
		rx := mk(verb, "rx", "B/s", 0, func(c *Ctx) (float64, error) {
			return c.rate(func(s *Snapshot) (float64, error) { return s.NetRx, s.NetErr })
		})
		tx := mk(verb, "tx", "B/s", 0, func(c *Ctx) (float64, error) {
			return c.rate(func(s *Snapshot) (float64, error) { return s.NetTx, s.NetErr })
		})
		rx.Rate, tx.Rate, rx.Probe, tx.Probe = true, true, probe, probe
		return []Elem{rx, tx}
	}
	return nil
}

/** All returns every reading element, verbs in `rptop list` order.
 *
 * Parameters:
 *   src (collector.Sources) — used to count the cores
 *
 * Returns:
 *   []Elem — temp, cpu, mem, fan, volts, disk, net elements
 *
 * Example:
 *   n := len(element.All(env.Sources))
 */
func All(src collector.Sources) []Elem {
	var out []Elem
	for _, v := range Verbs {
		out = append(out, Group(v, src)...)
	}
	return out
}
