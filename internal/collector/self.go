package collector

import (
	"fmt"
	"strconv"
	"strings"
	"time"
)

// clockTick is the kernel's USER_HZ, 100 on every Linux the Pi runs.
const clockTick = 100

/** SelfTicks returns rptop's own cumulative CPU time: user plus system, in
 * clock ticks of 1/100 second, from /proc/self/stat.
 *
 * Returns:
 *   float64 — ticks since the process started
 *   error   — wrapped collector class
 *
 * Example:
 *   a, _ := collector.SelfTicks(s)
 */
func SelfTicks(s Sources) (float64, error) {
	txt, err := readText(s.SelfStat)
	if err != nil {
		return 0, err
	}
	// The name is in parentheses and may contain spaces and ")": the fields
	// start after the last ")".
	i := strings.LastIndexByte(txt, ')')
	if i < 0 {
		return 0, fmt.Errorf("%s: %w: no process name", s.SelfStat, ErrUnparseable)
	}
	f := strings.Fields(txt[i+1:])
	// f[0] is the state (field 3); utime is field 14 and stime field 15.
	if len(f) < 13 {
		return 0, fmt.Errorf("%s: %w: %d fields", s.SelfStat, ErrUnparseable, len(f))
	}
	u, e1 := strconv.ParseFloat(f[11], 64)
	st, e2 := strconv.ParseFloat(f[12], 64)
	if e1 != nil || e2 != nil {
		return 0, fmt.Errorf("%s: %w: utime/stime", s.SelfStat, ErrUnparseable)
	}
	return u + st, nil
}

/** SelfRSS returns rptop's resident memory from /proc/self/status.
 *
 * Returns:
 *   float64 — bytes
 *   error   — wrapped collector class
 *
 * Example:
 *   b, _ := collector.SelfRSS(s)
 */
func SelfRSS(s Sources) (float64, error) {
	txt, err := readText(s.SelfStatus)
	if err != nil {
		return 0, err
	}
	for _, line := range strings.Split(txt, "\n") {
		k, rest, ok := strings.Cut(line, ":")
		if !ok || k != "VmRSS" {
			continue
		}
		f := strings.Fields(rest)
		if len(f) == 0 {
			break
		}
		v, perr := strconv.ParseFloat(f[0], 64)
		if perr != nil {
			break
		}
		if len(f) > 1 && f[1] == "kB" {
			v *= 1024
		}
		return v, nil
	}
	return 0, fmt.Errorf("%s: %w: no VmRSS", s.SelfStatus, ErrUnparseable)
}

/** SelfPercent turns two SelfTicks values into rptop's CPU use as a percent of
 * one core.
 *
 * Parameters:
 *   prev, cur (float64) — earlier and later cumulative ticks
 *   dt (time.Duration)  — time between them
 *
 * Returns:
 *   float64 — percent of one core
 *   error   — ErrUnparseable if dt is not positive or the counter went backwards
 *
 * Example:
 *   p, err := collector.SelfPercent(200, 250, 2*time.Second) // 25
 */
func SelfPercent(prev, cur float64, dt time.Duration) (float64, error) {
	if dt <= 0 || cur < prev {
		return 0, fmt.Errorf("self cpu: %w: interval %v, %v to %v", ErrUnparseable, dt, prev, cur)
	}
	return (cur - prev) / clockTick / dt.Seconds() * 100, nil
}
