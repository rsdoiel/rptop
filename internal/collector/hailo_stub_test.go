//go:build !hailo

package collector

import (
	"errors"
	"testing"
)

// Without the hailo tag the element is reported absent (exit 66).
func TestOpenHailoStubIsAbsent(t *testing.T) {
	r, err := OpenHailo()
	if !errors.Is(err, ErrAbsent) {
		t.Errorf("err = %v, want ErrAbsent", err)
	}
	if r != nil {
		t.Errorf("reader = %v, want nil", r)
	}
}
