package collector

import (
	"errors"
	"fmt"
	"testing"
)

type fakeHailo struct {
	t   HailoTemps
	err error
}

func (f fakeHailo) Temps() (HailoTemps, error) { return f.t, f.err }
func (f fakeHailo) Close() error               { return nil }

func TestTempHailoTakesHigherSensor(t *testing.T) {
	got, err := TempHailo(fakeHailo{t: HailoTemps{TS0: 54.3, TS1: 53.9}})
	if err != nil {
		t.Fatal(err)
	}
	approx(t, "hailo", got, 54.3)
	got, _ = TempHailo(fakeHailo{t: HailoTemps{TS0: 50, TS1: 55}})
	approx(t, "hailo ts1", got, 55)
}

func TestTempHailoPassesErrorClass(t *testing.T) {
	_, err := TempHailo(fakeHailo{err: fmt.Errorf("read: %w", ErrUnreachable)})
	if !errors.Is(err, ErrUnreachable) {
		t.Errorf("err = %v, want ErrUnreachable", err)
	}
}

func TestTempHailoNilReader(t *testing.T) {
	if _, err := TempHailo(nil); !errors.Is(err, ErrAbsent) {
		t.Errorf("nil reader: err = %v, want ErrAbsent", err)
	}
}
