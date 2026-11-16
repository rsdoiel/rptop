package collector

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

const fixtures = "../../testdata/pi5-trixie"

// fixtureSources points every source at the captured Pi 5 files.
func fixtureSources(t *testing.T) Sources {
	t.Helper()
	abs, err := filepath.Abs(fixtures)
	if err != nil {
		t.Fatal(err)
	}
	return Sources{
		ThermalZone: filepath.Join(abs, "thermal_zone0_temp"),
		HwmonDir:    filepath.Join(abs, "hwmon"),
		CPUFreq:     filepath.Join(abs, "scaling_cur_freq"),
		LoadAvg:     filepath.Join(abs, "proc_loadavg"),
		MemInfo:     filepath.Join(abs, "proc_meminfo_head"),
		Stat:        filepath.Join(abs, "proc_stat_cpus"),
		DiskStats:   filepath.Join(abs, "proc_diskstats"),
		NetDev:      filepath.Join(abs, "proc_net_dev"),
		Vcgencmd: func(args ...string) ([]byte, error) {
			name := "vcgencmd"
			for _, a := range args {
				name += "_" + a
			}
			return os.ReadFile(filepath.Join(abs, name))
		},
	}
}

func approx(t *testing.T, name string, got, want float64) {
	t.Helper()
	if d := got - want; d > 1e-6 || d < -1e-6 {
		t.Errorf("%s = %v, want %v", name, got, want)
	}
}

func TestPlainCollectors(t *testing.T) {
	s := fixtureSources(t)
	cases := []struct {
		name string
		fn   func(Sources) (float64, error)
		want float64
	}{
		{"temp.soc", func(s Sources) (float64, error) { return TempSoC(s) }, 55.1},
		{"temp.rp1", func(s Sources) (float64, error) { return TempRP1(s) }, 55.472},
		{"temp.pmic", func(s Sources) (float64, error) { return TempPMIC(s) }, 54.3},
		{"fan", func(s Sources) (float64, error) { return FanRPM(s) }, 3725},
		{"volts.core", func(s Sources) (float64, error) { return VoltsCore(s) }, 0.7838},
		{"cpu.freq", func(s Sources) (float64, error) { return CPUFreqMHz(s) }, 1800},
		{"cpu.load", func(s Sources) (float64, error) { return Load1(s) }, 0.08},
		{"mem.total", func(s Sources) (float64, error) { return MemTotal(s) }, 16607552 * 1024},
		{"mem.used", func(s Sources) (float64, error) { return MemUsed(s) }, (16607552 - 14823632) * 1024},
	}
	for _, c := range cases {
		got, err := c.fn(s)
		if err != nil {
			t.Errorf("%s: %v", c.name, err)
			continue
		}
		approx(t, c.name, got, c.want)
	}
}

func TestAbsent(t *testing.T) {
	s := fixtureSources(t)
	s.HwmonDir = t.TempDir() // no devices
	s.Vcgencmd = nil         // no vcgencmd on this machine
	s.ThermalZone = filepath.Join(t.TempDir(), "nope")
	for name, err := range map[string]error{
		"fan":  second(FanRPM(s)),
		"pmic": second(TempPMIC(s)),
		"soc":  second(TempSoC(s)),
		"rp1":  second(TempRP1(s)),
	} {
		if !errors.Is(err, ErrAbsent) {
			t.Errorf("%s: err = %v, want ErrAbsent", name, err)
		}
	}
}

func TestUnparseable(t *testing.T) {
	s := fixtureSources(t)
	s.Vcgencmd = func(args ...string) ([]byte, error) { return []byte("garbage\n"), nil }
	if _, err := TempPMIC(s); !errors.Is(err, ErrUnparseable) {
		t.Errorf("pmic garbage: err = %v, want ErrUnparseable", err)
	}
	dir := t.TempDir()
	f := filepath.Join(dir, "temp")
	os.WriteFile(f, []byte("hot\n"), 0o644)
	s.ThermalZone = f
	if _, err := TempSoC(s); !errors.Is(err, ErrUnparseable) {
		t.Errorf("soc garbage: err = %v, want ErrUnparseable", err)
	}
}

func TestRefused(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("root ignores file modes")
	}
	s := fixtureSources(t)
	f := filepath.Join(t.TempDir(), "temp")
	os.WriteFile(f, []byte("55100\n"), 0o000)
	s.ThermalZone = f
	if _, err := TempSoC(s); !errors.Is(err, ErrRefused) {
		t.Errorf("err = %v, want ErrRefused", err)
	}
}

func second(_ float64, err error) error { return err }
