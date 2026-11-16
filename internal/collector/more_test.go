package collector

import (
	"errors"
	"path/filepath"
	"reflect"
	"testing"
	"time"
)

func TestCPUTimes(t *testing.T) {
	s := fixtureSources(t)
	all, err := CPUTimesOf(s, -1)
	if err != nil {
		t.Fatal(err)
	}
	approx(t, "total", all.Total, 28297235)
	approx(t, "busy", all.Busy, 24810) // total - idle - iowait
	c1, err := CPUTimesOf(s, 1)
	if err != nil {
		t.Fatal(err)
	}
	approx(t, "core1 busy", c1.Busy, 3624+0+2925+0+40+0-0)
	if n, err := CPUCores(s); err != nil || n != 4 {
		t.Errorf("CPUCores = %d, %v; want 4", n, err)
	}
	if _, err := CPUTimesOf(s, 9); !errors.Is(err, ErrAbsent) {
		t.Errorf("core 9: err = %v, want ErrAbsent", err)
	}
}

func TestCPUPercent(t *testing.T) {
	a := CPUTimes{Busy: 100, Total: 1000}
	b := CPUTimes{Busy: 150, Total: 1100}
	got, err := CPUPercent(a, b)
	if err != nil {
		t.Fatal(err)
	}
	approx(t, "percent", got, 50)
	if _, err := CPUPercent(a, a); !errors.Is(err, ErrUnparseable) {
		t.Errorf("no elapsed time: err = %v, want ErrUnparseable", err)
	}
}

func TestSwap(t *testing.T) {
	s := fixtureSources(t)
	s.MemInfo = filepath.Join(filepath.Dir(s.LoadAvg), "proc_meminfo_swap")
	total, err := SwapTotal(s)
	if err != nil {
		t.Fatal(err)
	}
	approx(t, "swap total", total, 2097136*1024)
	used, err := SwapUsed(s)
	if err != nil {
		t.Fatal(err)
	}
	approx(t, "swap used", used, 0)
}

func TestDiskCountsWholeDevicesOnly(t *testing.T) {
	r, w, err := DiskBytes(fixtureSources(t))
	if err != nil {
		t.Fatal(err)
	}
	// mmcblk0 + sda; partitions and zram0 are not counted.
	approx(t, "read", r, (2523237+13289)*512)
	approx(t, "write", w, 5661883*512)
}

func TestNetBytesSkipsLoopback(t *testing.T) {
	rx, tx, err := NetBytes(fixtureSources(t))
	if err != nil {
		t.Fatal(err)
	}
	approx(t, "rx", rx, 528478125)
	approx(t, "tx", tx, 10531085)
}

func TestRate(t *testing.T) {
	got, err := Rate(1000, 3000, 2*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	approx(t, "rate", got, 1000)
	if _, err := Rate(3000, 1000, time.Second); !errors.Is(err, ErrUnparseable) {
		t.Errorf("counter went back: err = %v", err)
	}
	if _, err := Rate(1, 2, 0); !errors.Is(err, ErrUnparseable) {
		t.Errorf("zero interval: err = %v", err)
	}
}

func TestThrottled(t *testing.T) {
	th, err := Throttled(fixtureSources(t))
	if err != nil {
		t.Fatal(err)
	}
	if got := th.Now(); len(got) != 0 {
		t.Errorf("Now() = %v, want none", got)
	}
	want := []string{"arm frequency capped", "throttled", "soft temperature limit"}
	if got := th.SinceBoot(); !reflect.DeepEqual(got, want) {
		t.Errorf("SinceBoot() = %v, want %v", got, want)
	}
	active := Throttle{Raw: 0x5}
	if got := active.Now(); !reflect.DeepEqual(got, []string{"under-voltage", "throttled"}) {
		t.Errorf("Now() = %v", got)
	}
}

func TestThrottledErrors(t *testing.T) {
	s := fixtureSources(t)
	s.Vcgencmd = func(...string) ([]byte, error) { return []byte("throttled=zzz\n"), nil }
	if _, err := Throttled(s); !errors.Is(err, ErrUnparseable) {
		t.Errorf("err = %v, want ErrUnparseable", err)
	}
	s.Vcgencmd = nil
	if _, err := Throttled(s); !errors.Is(err, ErrAbsent) {
		t.Errorf("err = %v, want ErrAbsent", err)
	}
}

func TestUnreachable(t *testing.T) {
	s := fixtureSources(t)
	// A directory where a file is expected: present, but cannot be read.
	s.CPUFreq = t.TempDir()
	if _, err := CPUFreqMHz(s); !errors.Is(err, ErrUnreachable) {
		t.Errorf("directory as file: err = %v, want ErrUnreachable", err)
	}
	s.Vcgencmd = func(...string) ([]byte, error) { return nil, errors.New("vcgencmd exited 1") }
	if _, err := TempPMIC(s); !errors.Is(err, ErrUnreachable) {
		t.Errorf("vcgencmd failure: err = %v, want ErrUnreachable", err)
	}
}
