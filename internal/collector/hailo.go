package collector

import (
	"fmt"
	"strconv"
)

/** HailoTemps holds the two on-chip temperature sensors of a Hailo device.
 *
 * Fields:
 *   TS0 (float64) — first sensor, degrees Celsius
 *   TS1 (float64) — second sensor, degrees Celsius
 *
 * Example:
 *   t := collector.HailoTemps{TS0: 54.3, TS1: 53.9}
 */
type HailoTemps struct{ TS0, TS1 float64 }

/** HailoReader is an open Hailo device that can report its temperatures.
 * The device is opened once with OpenHailo and released with Close (DR-0001).
 *
 * Example:
 *   r, err := collector.OpenHailo()
 *   defer r.Close()
 */
type HailoReader interface {
	Temps() (HailoTemps, error)
	Close() error
}

/** TempHailo returns the Hailo chip temperature, the higher of the two
 * sensors.
 *
 * Parameters:
 *   r (HailoReader) — an open device, or nil if there is none
 *
 * Returns:
 *   float64 — degrees Celsius
 *   error   — ErrAbsent for a nil reader; otherwise the reader's error class
 *
 * Example:
 *   c, err := collector.TempHailo(r) // 54.3
 */
func TempHailo(r HailoReader) (float64, error) {
	if r == nil {
		return 0, fmt.Errorf("hailo: %w", ErrAbsent)
	}
	t, err := r.Temps()
	if err != nil {
		return 0, err
	}
	if t.TS1 > t.TS0 {
		return t.TS1, nil
	}
	return t.TS0, nil
}

// widen32 converts a float32 reading to float64 through its shortest decimal
// form, so 54.616 stays 54.616 and does not become 54.61600112915039.
func widen32(v float32) float64 {
	f, err := strconv.ParseFloat(strconv.FormatFloat(float64(v), 'f', -1, 32), 64)
	if err != nil {
		return float64(v)
	}
	return f
}
