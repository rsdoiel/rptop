package ui

import (
	"fmt"
	"math"
	"strings"
)

// Level is how a value is coloured: DR-0004 6.
type Level int

// The colour levels. Dim is for a value that is not available.
const (
	Green Level = iota
	Yellow
	Red
	Dim
)

func (l Level) String() string { return [...]string{"green", "yellow", "red", "dim"}[l] }

/** TempLevel decides a temperature's colour (DR-0004 6): green below warn,
 * yellow from warn up to crit, red from crit.
 *
 * Parameters:
 *   v, warn, crit (float64) — the reading and the two thresholds, in °C
 *
 * Returns:
 *   Level — Green, Yellow or Red
 *
 * Example:
 *   ui.TempLevel(65, 60, 80) // ui.Yellow
 */
func TempLevel(v, warn, crit float64) Level {
	switch {
	case v >= crit:
		return Red
	case v >= warn:
		return Yellow
	}
	return Green
}

/** ThrottleLevel decides the throttle colour: red when any flag is active now,
 * yellow when flags are set only since boot, green when none.
 *
 * Parameters:
 *   now, sinceBoot (float64) — the two flag words of the sample
 *
 * Returns:
 *   Level — Green, Yellow or Red
 *
 * Example:
 *   ui.ThrottleLevel(0, 14) // ui.Yellow
 */
func ThrottleLevel(now, sinceBoot float64) Level {
	switch {
	case now != 0:
		return Red
	case sinceBoot != 0:
		return Yellow
	}
	return Green
}

var blocks = []rune("▁▂▃▄▅▆▇█")

/** Spark draws the last width samples as block characters (DR-0004 5). The
 * scale runs from the window's minimum to its maximum but never over less than
 * minSpan, so a steady value is a flat line. A nil sample is a gap.
 *
 * Parameters:
 *   vals    ([]*float64) — the history, oldest first
 *   minSpan (float64)    — the narrowest range the scale may cover
 *   width   (int)        — the most characters to draw
 *
 * Returns:
 *   string — one character per sample, "" for no history
 *
 * Example:
 *   ui.Spark(hist, 5, 40) // "▁▂▃▅▆█"
 */
func Spark(vals []*float64, minSpan float64, width int) string {
	if width > 0 && len(vals) > width {
		vals = vals[len(vals)-width:]
	}
	lo, hi, any := math.Inf(1), math.Inf(-1), false
	for _, v := range vals {
		if v != nil {
			any = true
			lo, hi = math.Min(lo, *v), math.Max(hi, *v)
		}
	}
	if !any {
		return strings.Repeat(" ", len(vals))
	}
	if hi-lo < minSpan {
		hi = lo + minSpan
	}
	var b strings.Builder
	for _, v := range vals {
		if v == nil {
			b.WriteByte(' ')
			continue
		}
		i := int(math.Round((*v - lo) / (hi - lo) * float64(len(blocks)-1)))
		b.WriteRune(blocks[min(max(i, 0), len(blocks)-1)])
	}
	return b.String()
}

func scale(v float64, units []string, step float64) (float64, string) {
	i := 0
	for v >= step && i < len(units)-1 {
		v /= step
		i++
	}
	return v, units[i]
}

/** Rate formats bytes per second in binary units: "0 B/s", "1.5 KB/s", "12.0 MB/s".
 *
 * Parameters:
 *   v (float64) — bytes per second
 *
 * Returns:
 *   string — the text
 *
 * Example:
 *   ui.Rate(1536) // "1.5 KB/s"
 */
func Rate(v float64) string {
	s, u := scale(v, []string{"B", "KB", "MB", "GB", "TB"}, 1024)
	if u == "B" {
		return fmt.Sprintf("%.0f B/s", s)
	}
	return fmt.Sprintf("%.1f %s/s", s, u)
}

/** Bytes formats a size in binary units: "0 B", "512K", "2.1G", "15.8G".
 *
 * Parameters:
 *   v (float64) — bytes
 *
 * Returns:
 *   string — the text
 *
 * Example:
 *   ui.Bytes(2254857830) // "2.1G"
 */
func Bytes(v float64) string {
	s, u := scale(v, []string{"B", "K", "M", "G", "T"}, 1024)
	switch {
	case u == "B":
		return fmt.Sprintf("%.0f B", s)
	case s >= 100:
		return fmt.Sprintf("%.0f%s", s, u)
	}
	return fmt.Sprintf("%.1f%s", s, u)
}

// bar draws a fraction (0 to 1) as [████░░░░] of the given inner width.
func bar(frac float64, width int) string {
	frac = math.Min(math.Max(frac, 0), 1)
	n := int(math.Round(frac * float64(width)))
	return "[" + strings.Repeat("█", n) + strings.Repeat("░", width-n) + "]"
}

// uptimeText is "up 1d 10h", "up 3h 12m" or "up 5m".
func uptimeText(sec int) string {
	d, h, m := sec/86400, sec%86400/3600, sec%3600/60
	switch {
	case d > 0:
		return fmt.Sprintf("up %dd %dh", d, h)
	case h > 0:
		return fmt.Sprintf("up %dh %dm", h, m)
	}
	return fmt.Sprintf("up %dm", m)
}
