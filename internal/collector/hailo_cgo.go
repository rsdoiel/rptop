//go:build hailo

package collector

/*
#cgo LDFLAGS: -lhailort
#include <hailo/hailort.h>
#include <stdlib.h>

static hailo_status rptop_open(hailo_device *dev) {
	return hailo_create_device_by_id(NULL, dev);
}
static hailo_status rptop_temps(hailo_device dev, float *ts0, float *ts1) {
	hailo_chip_temperature_info_t info;
	hailo_status st = hailo_get_chip_temperature(dev, &info);
	if (st == HAILO_SUCCESS) {
		*ts0 = info.ts0_temperature;
		*ts1 = info.ts1_temperature;
	}
	return st;
}
// hailo_create_pcie_device is Hailo-8 only since 5.4.0, so the device is
// opened by id (NULL: any device).
// rptop_absent is 1 for the statuses that mean "no such device here".
static int rptop_absent(hailo_status st) {
	return st == HAILO_NOT_FOUND || st == HAILO_DEVICE_NOT_CONNECTED ||
		st == HAILO_PCIE_NOT_SUPPORTED_ON_PLATFORM;
}
*/
import "C"

import (
	"fmt"
	"sync"
)

type hailoDevice struct {
	mu  sync.Mutex
	dev C.hailo_device
}

// hailoErr wraps a non-success status in the class that describes it.
func hailoErr(what string, st C.hailo_status) error {
	if C.rptop_absent(st) != 0 {
		return fmt.Errorf("hailo %s: status %d: %w", what, int(st), ErrAbsent)
	}
	return fmt.Errorf("hailo %s: status %d: %w", what, int(st), ErrUnreachable)
}

/** OpenHailo opens the first Hailo device through libhailort.
 *
 * Returns:
 *   HailoReader — the open device; the caller must Close it
 *   error       — ErrAbsent when no device is found, ErrUnreachable when it cannot be opened
 *
 * Example:
 *   r, err := collector.OpenHailo()
 */
func OpenHailo() (HailoReader, error) {
	var dev C.hailo_device
	if st := C.rptop_open(&dev); st != C.HAILO_SUCCESS {
		return nil, hailoErr("open", st)
	}
	return &hailoDevice{dev: dev}, nil
}

// Temps reads both on-chip sensors.
func (h *hailoDevice) Temps() (HailoTemps, error) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.dev == nil {
		return HailoTemps{}, fmt.Errorf("hailo: device closed: %w", ErrUnreachable)
	}
	var a, b C.float
	if st := C.rptop_temps(h.dev, &a, &b); st != C.HAILO_SUCCESS {
		return HailoTemps{}, hailoErr("temperature", st)
	}
	return HailoTemps{TS0: widen32(float32(a)), TS1: widen32(float32(b))}, nil
}

// Close releases the device; it is safe to call twice.
func (h *hailoDevice) Close() error {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.dev == nil {
		return nil
	}
	st := C.hailo_release_device(h.dev)
	h.dev = nil
	if st != C.HAILO_SUCCESS {
		return hailoErr("release", st)
	}
	return nil
}
