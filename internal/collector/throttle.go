package collector

import (
	"fmt"
	"strconv"
)

// Flag bits of vcgencmd get_throttled; the "since boot" bits are the "now"
// bits shifted left by 16.
var throttleFlags = [4]string{"under-voltage", "arm frequency capped", "throttled", "soft temperature limit"}

/** Throttle is the raw get_throttled word.
 *
 * Fields:
 *   Raw (uint32) — bits 0-3 are active now, bits 16-19 have occurred since boot
 *
 * Example:
 *   th, _ := collector.Throttled(s)
 *   th.Now() // []string{} when nothing is active
 */
type Throttle struct{ Raw uint32 }

func (t Throttle) decode(shift uint) []string {
	out := []string{}
	for i, name := range throttleFlags {
		if t.Raw&(1<<(uint(i)+shift)) != 0 {
			out = append(out, name)
		}
	}
	return out
}

/** Now lists the conditions active at this moment.
 *
 * Returns:
 *   []string — flag names, empty when none
 *
 * Example:
 *   Throttle{Raw: 0x5}.Now() // under-voltage, throttled
 */
func (t Throttle) Now() []string { return t.decode(0) }

/** SinceBoot lists the conditions that have occurred since boot.
 *
 * Returns:
 *   []string — flag names, empty when none
 *
 * Example:
 *   Throttle{Raw: 0xe0000}.SinceBoot() // arm frequency capped, throttled, soft temperature limit
 */
func (t Throttle) SinceBoot() []string { return t.decode(16) }

/** Throttled reads and decodes vcgencmd get_throttled.
 *
 * Returns:
 *   Throttle — the decoded word
 *   error    — ErrAbsent without vcgencmd, ErrUnparseable on a bad word
 *
 * Example:
 *   th, err := collector.Throttled(collector.DefaultSources())
 */
func Throttled(s Sources) (Throttle, error) {
	txt, val, err := vcgencmdRaw(s, "get_throttled")
	if err != nil {
		return Throttle{}, err
	}
	v, perr := strconv.ParseUint(val, 0, 32)
	if perr != nil {
		return Throttle{}, fmt.Errorf("vcgencmd get_throttled: %w: %q", ErrUnparseable, txt)
	}
	return Throttle{Raw: uint32(v)}, nil
}
