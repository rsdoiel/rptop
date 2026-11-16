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

// libhailort reports float32; widening it naively gives 54.61600112915039.
func TestWiden32KeepsTheShortestDecimal(t *testing.T) {
	for _, v := range []float32{54.616, 53.4, 0.1, 61.25} {
		got := widen32(v)
		want := map[float32]float64{54.616: 54.616, 53.4: 53.4, 0.1: 0.1, 61.25: 61.25}[v]
		if got != want {
			t.Errorf("widen32(%v) = %v, want %v", v, got, want)
		}
	}
}
