package cli

import (
	"fmt"
	"strconv"
	"time"

	"github.com/rsdoiel/rptop/internal/collector"
)

/** Env is the machine rptop reads: where the sensors are, how to open the
 * Hailo device and how to wait. Tests inject fixtures; DefaultEnv is the real
 * Pi.
 *
 * Fields:
 *   Sources   (collector.Sources)                 — sensor paths and the vcgencmd runner
 *   OpenHailo (func() (collector.HailoReader, error)) — opens the Hailo device; nil means absent
 *   Sleep     (func(time.Duration))               — waits between the two samples of a rate; nil means time.Sleep
 *
 * Example:
 *   env := cli.DefaultEnv()
 */
type Env struct {
	Sources   collector.Sources
	OpenHailo func() (collector.HailoReader, error)
	Sleep     func(time.Duration)
}

/** DefaultEnv returns the Env for the running Pi.
 *
 * Returns:
 *   Env — real /sys and /proc sources, collector.OpenHailo, time.Sleep
 *
 * Example:
 *   code := cli.RunEnv(cli.DefaultEnv(), args, os.Stdout, os.Stderr)
 */
func DefaultEnv() Env {
	return Env{Sources: collector.DefaultSources(), OpenHailo: collector.OpenHailo, Sleep: time.Sleep}
}

// elem is one element of a verb's group.
type elem struct {
	short  string
	unit   string
	digits int
	read   func(*ctx) (float64, error)
	probe  func(*ctx) error // availability without waiting; nil means read
}

// rateSet holds the two samples of one rate source and the first error.
type rateSet struct {
	done bool
	err  error
}

// ctx is the state of one command: the environment, the sampling interval,
// the lazily opened Hailo device and the two-sample caches.
type ctx struct {
	env      Env
	interval time.Duration

	hailo      collector.HailoReader
	hailoErr   error
	hailoTried bool

	cpuA, cpuB   map[int]collector.CPUTimes
	cpuErr       map[int]error
	cpuSet       rateSet
	diskR, diskW float64
	diskSet      rateSet
	netRx, netTx float64
	netSet       rateSet
}

func (c *ctx) sleep() {
	if c.env.Sleep != nil {
		c.env.Sleep(c.interval)
		return
	}
	time.Sleep(c.interval)
}

func (c *ctx) closeAll() {
	if c.hailo != nil {
		c.hailo.Close()
		c.hailo = nil
	}
}

// hailoTemp opens the Hailo device the first time it is needed and keeps it
// for the rest of the command.
func (c *ctx) hailoTemp() (float64, error) {
	if !c.hailoTried {
		c.hailoTried = true
		if c.env.OpenHailo == nil {
			c.hailoErr = fmt.Errorf("hailo: %w", collector.ErrAbsent)
		} else {
			c.hailo, c.hailoErr = c.env.OpenHailo()
		}
	}
	if c.hailoErr != nil {
		return 0, c.hailoErr
	}
	return collector.TempHailo(c.hailo)
}

// cpuPercent samples every core, waits one interval and samples again, once
// per command; each call then reads its own core from the cached pair.
func (c *ctx) cpuPercent(core int) (float64, error) {
	if !c.cpuSet.done {
		c.cpuSet.done = true
		c.cpuA, c.cpuB, c.cpuErr = map[int]collector.CPUTimes{}, map[int]collector.CPUTimes{}, map[int]error{}
		n, err := collector.CPUCores(c.env.Sources)
		keys := []int{-1}
		if err == nil {
			for i := 0; i < n; i++ {
				keys = append(keys, i)
			}
		}
		take := func(m map[int]collector.CPUTimes) {
			for _, k := range keys {
				t, e := collector.CPUTimesOf(c.env.Sources, k)
				if e != nil && c.cpuErr[k] == nil {
					c.cpuErr[k] = e
				}
				m[k] = t
			}
		}
		take(c.cpuA)
		c.sleep()
		take(c.cpuB)
	}
	if e := c.cpuErr[core]; e != nil {
		return 0, e
	}
	a, ok := c.cpuA[core]
	if !ok {
		return 0, fmt.Errorf("no such core: %w", collector.ErrAbsent)
	}
	return collector.CPUPercent(a, c.cpuB[core])
}

func (c *ctx) diskRate(write bool) (float64, error) {
	if !c.diskSet.done {
		c.diskSet.done = true
		r0, w0, err := collector.DiskBytes(c.env.Sources)
		c.sleep()
		r1, w1, err2 := collector.DiskBytes(c.env.Sources)
		if err == nil {
			err = err2
		}
		if err == nil {
			c.diskR, err = collector.Rate(r0, r1, c.interval)
		}
		if err == nil {
			c.diskW, err = collector.Rate(w0, w1, c.interval)
		}
		c.diskSet.err = err
	}
	if c.diskSet.err != nil {
		return 0, c.diskSet.err
	}
	if write {
		return c.diskW, nil
	}
	return c.diskR, nil
}

func (c *ctx) netRate(tx bool) (float64, error) {
	if !c.netSet.done {
		c.netSet.done = true
		r0, t0, err := collector.NetBytes(c.env.Sources)
		c.sleep()
		r1, t1, err2 := collector.NetBytes(c.env.Sources)
		if err == nil {
			err = err2
		}
		if err == nil {
			c.netRx, err = collector.Rate(r0, r1, c.interval)
		}
		if err == nil {
			c.netTx, err = collector.Rate(t0, t1, c.interval)
		}
		c.netSet.err = err
	}
	if c.netSet.err != nil {
		return 0, c.netSet.err
	}
	if tx {
		return c.netTx, nil
	}
	return c.netRx, nil
}

func plain(f func(collector.Sources) (float64, error)) func(*ctx) (float64, error) {
	return func(c *ctx) (float64, error) { return f(c.env.Sources) }
}

// verbOrder is the order of the groups in `list`.
var verbOrder = []string{"temp", "cpu", "mem", "fan", "volts", "disk", "net"}

// rateVerbs take -interval because their readings are two samples apart.
var rateVerbs = map[string]bool{"cpu": true, "disk": true, "net": true}

// coreElem is the element for core N of the cpu verb.
func coreElem(n int) elem {
	return elem{
		short: "core" + strconv.Itoa(n), unit: "%", digits: 1,
		read: func(c *ctx) (float64, error) { return c.cpuPercent(n) },
		probe: func(c *ctx) error {
			_, err := collector.CPUTimesOf(c.env.Sources, n)
			return err
		},
	}
}

// elementsOf returns the group of a reading verb, in report order.
func elementsOf(verb string, c *ctx) []elem {
	switch verb {
	case "temp":
		return []elem{
			{short: "soc", unit: "°C", digits: 1, read: plain(collector.TempSoC)},
			{short: "pmic", unit: "°C", digits: 1, read: plain(collector.TempPMIC)},
			{short: "hailo", unit: "°C", digits: 1, read: func(c *ctx) (float64, error) { return c.hailoTemp() }},
			{short: "rp1", unit: "°C", digits: 1, read: plain(collector.TempRP1)},
		}
	case "cpu":
		out := []elem{{
			short: "total", unit: "%", digits: 1,
			read: func(c *ctx) (float64, error) { return c.cpuPercent(-1) },
			probe: func(c *ctx) error {
				_, err := collector.CPUTimesOf(c.env.Sources, -1)
				return err
			},
		}}
		if n, err := collector.CPUCores(c.env.Sources); err == nil {
			for i := 0; i < n; i++ {
				out = append(out, coreElem(i))
			}
		}
		return append(out,
			elem{short: "freq", unit: "MHz", read: plain(collector.CPUFreqMHz)},
			elem{short: "load", unit: "", digits: 2, read: plain(collector.Load1)})
	case "mem":
		return []elem{
			{short: "used", unit: "bytes", read: plain(collector.MemUsed)},
			{short: "total", unit: "bytes", read: plain(collector.MemTotal)},
			{short: "swap", unit: "bytes", read: plain(collector.SwapUsed)},
		}
	case "fan":
		return []elem{{short: "speed", unit: "rpm", read: plain(collector.FanRPM)}}
	case "volts":
		return []elem{{short: "core", unit: "V", digits: 4, read: plain(collector.VoltsCore)}}
	case "disk":
		probe := func(c *ctx) error { _, _, err := collector.DiskBytes(c.env.Sources); return err }
		return []elem{
			{short: "read", unit: "B/s", read: func(c *ctx) (float64, error) { return c.diskRate(false) }, probe: probe},
			{short: "write", unit: "B/s", read: func(c *ctx) (float64, error) { return c.diskRate(true) }, probe: probe},
		}
	case "net":
		probe := func(c *ctx) error { _, _, err := collector.NetBytes(c.env.Sources); return err }
		return []elem{
			{short: "rx", unit: "B/s", read: func(c *ctx) (float64, error) { return c.netRate(false) }, probe: probe},
			{short: "tx", unit: "B/s", read: func(c *ctx) (float64, error) { return c.netRate(true) }, probe: probe},
		}
	}
	return nil
}
