package collector

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"
)

/** CPUTimes is the cumulative busy and total jiffies of one CPU line.
 *
 * Fields:
 *   Busy  (float64) — total minus idle and iowait
 *   Total (float64) — every counted jiffy
 *
 * Example:
 *   a, _ := collector.CPUTimesOf(s, -1)
 */
type CPUTimes struct{ Busy, Total float64 }

/** CPUTimesOf reads one CPU's counters from /proc/stat.
 *
 * Parameters:
 *   s    (Sources) — where to read
 *   core (int)     — core number, or -1 for the all-cores "cpu" line
 *
 * Returns:
 *   CPUTimes — cumulative counters
 *   error    — ErrAbsent if the core does not exist, ErrUnparseable on a bad line
 *
 * Example:
 *   t, err := collector.CPUTimesOf(s, 2)
 */
func CPUTimesOf(s Sources, core int) (CPUTimes, error) {
	txt, err := readText(s.Stat)
	if err != nil {
		return CPUTimes{}, err
	}
	label := "cpu"
	if core >= 0 {
		label = "cpu" + strconv.Itoa(core)
	}
	for _, line := range strings.Split(txt, "\n") {
		f := strings.Fields(line)
		if len(f) == 0 || f[0] != label {
			continue
		}
		if len(f) < 6 {
			return CPUTimes{}, fmt.Errorf("%s: %w: %q", s.Stat, ErrUnparseable, line)
		}
		var total float64
		var vals []float64
		for _, x := range f[1:] {
			v, perr := strconv.ParseFloat(x, 64)
			if perr != nil {
				return CPUTimes{}, fmt.Errorf("%s: %w: %q", s.Stat, ErrUnparseable, line)
			}
			vals = append(vals, v)
			total += v
		}
		// user nice system idle iowait ...: idle is field 4, iowait field 5.
		return CPUTimes{Busy: total - vals[3] - vals[4], Total: total}, nil
	}
	return CPUTimes{}, fmt.Errorf("%s: %s: %w", s.Stat, label, ErrAbsent)
}

/** CPUCores counts the per-core lines in /proc/stat.
 *
 * Returns:
 *   int   — number of cores
 *   error — wrapped collector class
 *
 * Example:
 *   n, err := collector.CPUCores(s) // 4
 */
func CPUCores(s Sources) (int, error) {
	txt, err := readText(s.Stat)
	if err != nil {
		return 0, err
	}
	n := 0
	for _, line := range strings.Split(txt, "\n") {
		if f := strings.Fields(line); len(f) > 0 && len(f[0]) > 3 && strings.HasPrefix(f[0], "cpu") {
			n++
		}
	}
	if n == 0 {
		return 0, fmt.Errorf("%s: %w: no cpu lines", s.Stat, ErrUnparseable)
	}
	return n, nil
}

/** CPUPercent turns two samples of CPUTimes into a utilisation percentage.
 *
 * Parameters:
 *   a (CPUTimes) — the earlier sample
 *   b (CPUTimes) — the later sample
 *
 * Returns:
 *   float64 — percent busy, 0 to 100
 *   error   — ErrUnparseable if no time passed or the counters went backwards
 *
 * Example:
 *   p, err := collector.CPUPercent(a, b)
 */
func CPUPercent(a, b CPUTimes) (float64, error) {
	dt := b.Total - a.Total
	if dt <= 0 || b.Busy < a.Busy {
		return 0, fmt.Errorf("cpu: %w: counters did not advance", ErrUnparseable)
	}
	return 100 * (b.Busy - a.Busy) / dt, nil
}

/** SwapTotal returns the swap size.
 *
 * Returns:
 *   float64 — bytes
 *   error   — wrapped collector class
 *
 * Example:
 *   b, err := collector.SwapTotal(s)
 */
func SwapTotal(s Sources) (float64, error) { return memField(s, "SwapTotal") }

/** SwapUsed returns swap in use: SwapTotal minus SwapFree.
 *
 * Returns:
 *   float64 — bytes
 *   error   — wrapped collector class
 *
 * Example:
 *   b, err := collector.SwapUsed(s)
 */
func SwapUsed(s Sources) (float64, error) {
	total, err := memField(s, "SwapTotal")
	if err != nil {
		return 0, err
	}
	free, err := memField(s, "SwapFree")
	return total - free, err
}

// wholeDisk matches physical block devices, not partitions, ram, loop or zram.
var wholeDisk = regexp.MustCompile(`^(sd[a-z]+|nvme[0-9]+n[0-9]+|mmcblk[0-9]+)$`)

/** DiskBytes returns cumulative bytes read and written, summed over whole
 * physical disks (sd*, nvme*, mmcblk*); partitions, ram, loop and zram are
 * left out.
 *
 * Returns:
 *   read, written (float64) — cumulative bytes
 *   error                   — wrapped collector class
 *
 * Example:
 *   r, w, err := collector.DiskBytes(s)
 */
func DiskBytes(s Sources) (read, written float64, err error) {
	txt, err := readText(s.DiskStats)
	if err != nil {
		return 0, 0, err
	}
	for _, line := range strings.Split(txt, "\n") {
		f := strings.Fields(line)
		if len(f) < 10 || !wholeDisk.MatchString(f[2]) {
			continue
		}
		r, e1 := strconv.ParseFloat(f[5], 64)
		w, e2 := strconv.ParseFloat(f[9], 64)
		if e1 != nil || e2 != nil {
			return 0, 0, fmt.Errorf("%s: %w: %q", s.DiskStats, ErrUnparseable, line)
		}
		read += r * 512
		written += w * 512
	}
	return read, written, nil
}

/** NetBytes returns cumulative bytes received and sent over every interface
 * except loopback.
 *
 * Returns:
 *   rx, tx (float64) — cumulative bytes
 *   error            — wrapped collector class
 *
 * Example:
 *   rx, tx, err := collector.NetBytes(s)
 */
func NetBytes(s Sources) (rx, tx float64, err error) {
	txt, err := readText(s.NetDev)
	if err != nil {
		return 0, 0, err
	}
	for _, line := range strings.Split(txt, "\n") {
		name, rest, ok := strings.Cut(line, ":")
		if !ok || strings.TrimSpace(name) == "lo" {
			continue
		}
		f := strings.Fields(rest)
		if len(f) < 9 {
			return 0, 0, fmt.Errorf("%s: %w: %q", s.NetDev, ErrUnparseable, line)
		}
		r, e1 := strconv.ParseFloat(f[0], 64)
		t, e2 := strconv.ParseFloat(f[8], 64)
		if e1 != nil || e2 != nil {
			return 0, 0, fmt.Errorf("%s: %w: %q", s.NetDev, ErrUnparseable, line)
		}
		rx += r
		tx += t
	}
	return rx, tx, nil
}

/** Rate converts two cumulative counter values into a per-second rate.
 *
 * Parameters:
 *   prev (float64)       — earlier counter value
 *   cur  (float64)       — later counter value
 *   dt   (time.Duration) — time between the samples
 *
 * Returns:
 *   float64 — units per second
 *   error   — ErrUnparseable if dt is not positive or the counter went backwards
 *
 * Example:
 *   bps, err := collector.Rate(1000, 3000, 2*time.Second) // 1000
 */
func Rate(prev, cur float64, dt time.Duration) (float64, error) {
	if dt <= 0 || cur < prev {
		return 0, fmt.Errorf("rate: %w: interval %v, %v to %v", ErrUnparseable, dt, prev, cur)
	}
	return (cur - prev) / dt.Seconds(), nil
}
