// Package collector reads the Pi's sensors. Each collector takes a Sources
// value, so tests point it at captured fixtures and the program points it at
// the real /sys, /proc and vcgencmd.
package collector

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
)

// Error classes, each wrapped with %w; internal/cli maps them to exit codes
// (absent 66, unreachable 69, unparseable 65, refused 77).
var (
	ErrAbsent      = errors.New("not present on this machine")
	ErrUnreachable = errors.New("present but cannot be reached")
	ErrUnparseable = errors.New("cannot be parsed")
	ErrRefused     = errors.New("permission refused")
)

/** Sources names where each collector reads.
 *
 * Fields:
 *   ThermalZone (string) — file holding the SoC temperature in millidegrees
 *   HwmonDir    (string) — directory of hwmon devices, each with a name file
 *   CPUFreq     (string) — scaling_cur_freq, in kHz
 *   LoadAvg     (string) — /proc/loadavg
 *   MemInfo     (string) — /proc/meminfo
 *   Stat        (string) — /proc/stat
 *   DiskStats   (string) — /proc/diskstats
 *   NetDev      (string) — /proc/net/dev
 *   Vcgencmd    (func)   — runs vcgencmd with args; nil when it is not installed
 *
 * Example:
 *   s := collector.DefaultSources()
 */
type Sources struct {
	ThermalZone string
	HwmonDir    string
	CPUFreq     string
	LoadAvg     string
	MemInfo     string
	Stat        string
	DiskStats   string
	NetDev      string
	Vcgencmd    func(args ...string) ([]byte, error)
}

/** DefaultSources returns the Sources for the running Pi.
 *
 * Returns:
 *   Sources — real /sys and /proc paths; Vcgencmd is nil if vcgencmd is not on PATH
 *
 * Example:
 *   v, err := collector.TempSoC(collector.DefaultSources())
 */
func DefaultSources() Sources {
	s := Sources{
		ThermalZone: "/sys/class/thermal/thermal_zone0/temp",
		HwmonDir:    "/sys/class/hwmon",
		CPUFreq:     "/sys/devices/system/cpu/cpu0/cpufreq/scaling_cur_freq",
		LoadAvg:     "/proc/loadavg",
		MemInfo:     "/proc/meminfo",
		Stat:        "/proc/stat",
		DiskStats:   "/proc/diskstats",
		NetDev:      "/proc/net/dev",
	}
	if path, err := exec.LookPath("vcgencmd"); err == nil {
		s.Vcgencmd = func(args ...string) ([]byte, error) {
			return exec.Command(path, args...).Output()
		}
	}
	return s
}

// classify wraps a file error in the class that describes it.
func classify(path string, err error) error {
	switch {
	case errors.Is(err, fs.ErrNotExist):
		return fmt.Errorf("%s: %w", path, ErrAbsent)
	case errors.Is(err, fs.ErrPermission):
		return fmt.Errorf("%s: %w", path, ErrRefused)
	}
	return fmt.Errorf("%s: %w: %v", path, ErrUnreachable, err)
}

func readText(path string) (string, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return "", classify(path, err)
	}
	return strings.TrimSpace(string(b)), nil
}

func readFloat(path string) (float64, error) {
	txt, err := readText(path)
	if err != nil {
		return 0, err
	}
	v, err := strconv.ParseFloat(txt, 64)
	if err != nil {
		return 0, fmt.Errorf("%s: %w: %q", path, ErrUnparseable, txt)
	}
	return v, nil
}

// hwmonFile finds the hwmon device whose name file is name and returns the
// path of file inside it. hwmon numbers vary, so devices are found by name.
func hwmonFile(s Sources, name, file string) (string, error) {
	entries, err := os.ReadDir(s.HwmonDir)
	if err != nil {
		return "", classify(s.HwmonDir, err)
	}
	for _, e := range entries {
		dir := filepath.Join(s.HwmonDir, e.Name())
		if n, err := readText(filepath.Join(dir, "name")); err == nil && n == name {
			return filepath.Join(dir, file), nil
		}
	}
	return "", fmt.Errorf("hwmon %s: %w", name, ErrAbsent)
}

// vcgencmdValue runs vcgencmd and returns the number between "key=" and suffix.
func vcgencmdValue(s Sources, suffix string, args ...string) (float64, error) {
	txt, val, err := vcgencmdRaw(s, args...)
	if err != nil {
		return 0, err
	}
	v, perr := strconv.ParseFloat(strings.TrimSuffix(val, suffix), 64)
	if perr != nil {
		return 0, fmt.Errorf("vcgencmd %s: %w: %q", strings.Join(args, " "), ErrUnparseable, txt)
	}
	return v, nil
}

// vcgencmdRaw runs vcgencmd and returns its trimmed output and the text after "=".
func vcgencmdRaw(s Sources, args ...string) (txt, val string, err error) {
	if s.Vcgencmd == nil {
		return "", "", fmt.Errorf("vcgencmd: %w", ErrAbsent)
	}
	out, rerr := s.Vcgencmd(args...)
	if rerr != nil {
		switch {
		case errors.Is(rerr, fs.ErrNotExist) || errors.Is(rerr, exec.ErrNotFound):
			return "", "", fmt.Errorf("vcgencmd: %w", ErrAbsent)
		case errors.Is(rerr, fs.ErrPermission):
			return "", "", fmt.Errorf("vcgencmd: %w", ErrRefused)
		}
		return "", "", fmt.Errorf("vcgencmd %s: %w: %v", strings.Join(args, " "), ErrUnreachable, rerr)
	}
	txt = strings.TrimSpace(string(out))
	_, val, ok := strings.Cut(txt, "=")
	if !ok {
		return "", "", fmt.Errorf("vcgencmd %s: %w: %q", strings.Join(args, " "), ErrUnparseable, txt)
	}
	return txt, val, nil
}

/** TempSoC returns the SoC temperature.
 *
 * Returns:
 *   float64 — degrees Celsius
 *   error   — ErrAbsent, ErrRefused or ErrUnparseable, wrapped
 *
 * Example:
 *   c, err := collector.TempSoC(collector.DefaultSources()) // 55.1
 */
func TempSoC(s Sources) (float64, error) {
	v, err := readFloat(s.ThermalZone)
	return v / 1000, err
}

/** TempRP1 returns the RP1 chip temperature from the rp1_adc hwmon device.
 *
 * Returns:
 *   float64 — degrees Celsius
 *   error   — wrapped collector class
 *
 * Example:
 *   c, err := collector.TempRP1(collector.DefaultSources()) // 55.472
 */
func TempRP1(s Sources) (float64, error) {
	p, err := hwmonFile(s, "rp1_adc", "temp1_input")
	if err != nil {
		return 0, err
	}
	v, err := readFloat(p)
	return v / 1000, err
}

/** TempPMIC returns the power-management IC temperature via vcgencmd.
 *
 * Returns:
 *   float64 — degrees Celsius
 *   error   — wrapped collector class
 *
 * Example:
 *   c, err := collector.TempPMIC(collector.DefaultSources()) // 54.3
 */
func TempPMIC(s Sources) (float64, error) {
	return vcgencmdValue(s, "'C", "measure_temp", "pmic")
}

/** FanRPM returns the fan speed from the pwmfan hwmon device.
 *
 * Returns:
 *   float64 — revolutions per minute
 *   error   — wrapped collector class
 *
 * Example:
 *   rpm, err := collector.FanRPM(collector.DefaultSources()) // 3725
 */
func FanRPM(s Sources) (float64, error) {
	p, err := hwmonFile(s, "pwmfan", "fan1_input")
	if err != nil {
		return 0, err
	}
	return readFloat(p)
}

/** VoltsCore returns the core voltage via vcgencmd.
 *
 * Returns:
 *   float64 — volts
 *   error   — wrapped collector class
 *
 * Example:
 *   v, err := collector.VoltsCore(collector.DefaultSources()) // 0.7838
 */
func VoltsCore(s Sources) (float64, error) {
	return vcgencmdValue(s, "V", "measure_volts", "core")
}

/** CPUFreqMHz returns the current CPU frequency.
 *
 * Returns:
 *   float64 — megahertz
 *   error   — wrapped collector class
 *
 * Example:
 *   mhz, err := collector.CPUFreqMHz(collector.DefaultSources()) // 1800
 */
func CPUFreqMHz(s Sources) (float64, error) {
	v, err := readFloat(s.CPUFreq)
	return v / 1000, err
}

/** Load1 returns the one-minute load average.
 *
 * Returns:
 *   float64 — load average
 *   error   — wrapped collector class
 *
 * Example:
 *   l, err := collector.Load1(collector.DefaultSources()) // 0.08
 */
func Load1(s Sources) (float64, error) {
	txt, err := readText(s.LoadAvg)
	if err != nil {
		return 0, err
	}
	f := strings.Fields(txt)
	if len(f) == 0 {
		return 0, fmt.Errorf("%s: %w: empty", s.LoadAvg, ErrUnparseable)
	}
	v, perr := strconv.ParseFloat(f[0], 64)
	if perr != nil {
		return 0, fmt.Errorf("%s: %w: %q", s.LoadAvg, ErrUnparseable, txt)
	}
	return v, nil
}

// memField returns a /proc/meminfo field in bytes.
func memField(s Sources, key string) (float64, error) {
	txt, err := readText(s.MemInfo)
	if err != nil {
		return 0, err
	}
	for _, line := range strings.Split(txt, "\n") {
		k, rest, ok := strings.Cut(line, ":")
		if !ok || k != key {
			continue
		}
		f := strings.Fields(rest)
		if len(f) == 0 {
			break
		}
		v, perr := strconv.ParseFloat(f[0], 64)
		if perr != nil {
			break
		}
		if len(f) > 1 && f[1] == "kB" {
			v *= 1024
		}
		return v, nil
	}
	return 0, fmt.Errorf("%s: %w: no %s", s.MemInfo, ErrUnparseable, key)
}

/** MemTotal returns total memory.
 *
 * Returns:
 *   float64 — bytes
 *   error   — wrapped collector class
 *
 * Example:
 *   b, err := collector.MemTotal(collector.DefaultSources())
 */
func MemTotal(s Sources) (float64, error) { return memField(s, "MemTotal") }

/** MemUsed returns memory in use: MemTotal minus MemAvailable.
 *
 * Returns:
 *   float64 — bytes
 *   error   — wrapped collector class
 *
 * Example:
 *   b, err := collector.MemUsed(collector.DefaultSources())
 */
func MemUsed(s Sources) (float64, error) {
	total, err := memField(s, "MemTotal")
	if err != nil {
		return 0, err
	}
	avail, err := memField(s, "MemAvailable")
	return total - avail, err
}
