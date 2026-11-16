//go:build !hailo

package collector

import "fmt"

/** OpenHailo, in a build without the hailo tag, reports the device absent.
 *
 * Returns:
 *   HailoReader — always nil
 *   error       — ErrAbsent, wrapped
 *
 * Example:
 *   _, err := collector.OpenHailo() // ErrAbsent unless built with -tags hailo
 */
func OpenHailo() (HailoReader, error) {
	return nil, fmt.Errorf("hailo: built without the hailo tag: %w", ErrAbsent)
}
