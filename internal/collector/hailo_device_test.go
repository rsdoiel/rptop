//go:build hailo

package collector

import (
	"errors"
	"testing"
)

// Needs a Hailo device and libhailort. With none present it is reported
// absent and the test skips.
func TestOpenHailoDevice(t *testing.T) {
	r, err := OpenHailo()
	if errors.Is(err, ErrAbsent) {
		t.Skip("no Hailo device on this machine")
	}
	if err != nil {
		t.Fatal(err)
	}
	defer r.Close()
	temp, err := TempHailo(r)
	if err != nil {
		t.Fatal(err)
	}
	if temp < 10 || temp > 120 {
		t.Errorf("implausible Hailo temperature %v", temp)
	}
	// The handle is held open: a second read must still work.
	if _, err := TempHailo(r); err != nil {
		t.Errorf("second read: %v", err)
	}
}
