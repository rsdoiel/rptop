// Package testenv gives tests a copy of the captured Pi 5 files that they can
// change, so counters can be advanced between samples. It is used only by
// tests.
package testenv

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/rsdoiel/rptop/internal/collector"
	"github.com/rsdoiel/rptop/internal/element"
)

/** Fixture is a writable copy of testdata/pi5-trixie and an Env that reads it.
 * No Hailo device is attached: the element is absent unless a test sets
 * Env.OpenHailo.
 *
 * Fields:
 *   Dir (string)       — the temp directory holding the copy
 *   Env (element.Env)  — reads Dir; change Env.Sources freely in a test
 *
 * Example:
 *   f := testenv.New(t)
 *   f.Advance()
 */
type Fixture struct {
	t    *testing.T
	Dir  string
	Env  element.Env
	n    int
	orig map[string]string
}

/** New copies the fixtures from fixtureDir into a temp directory.
 *
 * Parameters:
 *   t (*testing.T)     — the test
 *   fixtureDir (string) — path of testdata/pi5-trixie relative to the test package
 *
 * Returns:
 *   *Fixture — with an Env wired to the copy
 *
 * Example:
 *   f := testenv.New(t, "../../testdata/pi5-trixie")
 */
func New(t *testing.T, fixtureDir string) *Fixture {
	t.Helper()
	f := &Fixture{t: t, Dir: t.TempDir(), orig: map[string]string{}}
	var copyTree func(src, dst string)
	copyTree = func(src, dst string) {
		entries, err := os.ReadDir(src)
		if err != nil {
			t.Fatal(err)
		}
		os.MkdirAll(dst, 0o755)
		for _, e := range entries {
			if e.IsDir() {
				copyTree(filepath.Join(src, e.Name()), filepath.Join(dst, e.Name()))
				continue
			}
			b, err := os.ReadFile(filepath.Join(src, e.Name()))
			if err != nil {
				t.Fatal(err)
			}
			os.WriteFile(filepath.Join(dst, e.Name()), b, 0o644)
		}
	}
	copyTree(fixtureDir, f.Dir)
	p := func(n string) string { return filepath.Join(f.Dir, n) }
	f.Env = element.Env{
		Sources: collector.Sources{
			ThermalZone: p("thermal_zone0_temp"),
			HwmonDir:    p("hwmon"),
			CPUFreq:     p("scaling_cur_freq"),
			LoadAvg:     p("proc_loadavg"),
			MemInfo:     p("proc_meminfo_swap"),
			Stat:        p("proc_stat_cpus"),
			DiskStats:   p("proc_diskstats"),
			NetDev:      p("proc_net_dev"),
			SelfStat:    p("proc_self_stat"),
			SelfStatus:  p("proc_self_status"),
			Vcgencmd: func(args ...string) ([]byte, error) {
				name := "vcgencmd"
				for _, a := range args {
					name += "_" + a
				}
				return os.ReadFile(p(name))
			},
		},
		OpenHailo: func() (collector.HailoReader, error) {
			return nil, fmt.Errorf("hailo: %w", collector.ErrAbsent)
		},
	}
	return f
}

// Sub replaces the first occurrence of old with new in a fixture file.
func (f *Fixture) Sub(file, old, new string) {
	f.t.Helper()
	p := filepath.Join(f.Dir, file)
	b, err := os.ReadFile(p)
	if err != nil {
		f.t.Fatal(err)
	}
	s := string(b)
	if !strings.Contains(s, old) {
		f.t.Fatalf("%s: %q not found", file, old)
	}
	os.WriteFile(p, []byte(strings.Replace(s, old, new, 1)), 0o644)
}

/** Advance moves every counter one more interval forward. Each call adds to
 * the original values: all cores gain 500 busy and 500 idle jiffies (so
 * cpu.total reads 50%), core1 gains 300 busy and 700 idle (30%), cores 0, 2 and
 * 3 gain idle time, mmcblk0 gains 1 MiB read, and wlan0 gains 1000 bytes
 * received.
 *
 * Example:
 *   f.Advance() // cpu.total is 50% over the interval, however many times it is called
 */
func (f *Fixture) Advance() {
	f.t.Helper()
	f.n++
	n := f.n
	rewrite := func(file string, pairs ...string) {
		orig, ok := f.orig[file]
		if !ok {
			b, err := os.ReadFile(filepath.Join(f.Dir, file))
			if err != nil {
				f.t.Fatal(err)
			}
			orig = string(b)
			f.orig[file] = orig
		}
		s := orig
		for i := 0; i < len(pairs); i += 2 {
			if !strings.Contains(s, pairs[i]) {
				f.t.Fatalf("%s: %q not found", file, pairs[i])
			}
			s = strings.Replace(s, pairs[i], pairs[i+1], 1)
		}
		os.WriteFile(filepath.Join(f.Dir, file), []byte(s), 0o644)
	}
	rewrite("proc_stat_cpus",
		"cpu  13006 16 11666 28261786", fmt.Sprintf("cpu  %d 16 11666 %d", 13006+500*n, 28261786+500*n),
		"cpu1 3624 0 2925 7064772", fmt.Sprintf("cpu1 %d 0 2925 %d", 3624+300*n, 7064772+700*n),
		"cpu0 3178 9 2514 7064147", fmt.Sprintf("cpu0 3178 9 2514 %d", 7064147+1000*n),
		"cpu2 2889 2 2917 7065913", fmt.Sprintf("cpu2 2889 2 2917 %d", 7065913+1000*n),
		"cpu3 3314 4 3308 7066952", fmt.Sprintf("cpu3 3314 4 3308 %d", 7066952+1000*n))
	rewrite("proc_diskstats", "mmcblk0 32942 28466 2523237", fmt.Sprintf("mmcblk0 32942 28466 %d", 2523237+2048*n))
	rewrite("proc_net_dev", "wlan0: 528478125", fmt.Sprintf("wlan0: %d", 528478125+1000*n))
}
