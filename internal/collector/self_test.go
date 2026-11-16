package collector

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestSelfFromFixtures(t *testing.T) {
	s := fixtureSources(t)
	s.SelfStat = filepath.Join(filepath.Dir(s.LoadAvg), "proc_self_stat")
	s.SelfStatus = filepath.Join(filepath.Dir(s.LoadAvg), "proc_self_status")
	ticks, err := SelfTicks(s)
	if err != nil {
		t.Fatal(err)
	}
	approx(t, "ticks", ticks, 200) // utime 150 + stime 50
	rss, err := SelfRSS(s)
	if err != nil {
		t.Fatal(err)
	}
	approx(t, "rss", rss, 18432*1024)
}

// The process name is in parentheses and may itself contain spaces and ")".
func TestSelfTicksSurvivesAnAwkwardName(t *testing.T) {
	s := fixtureSources(t)
	f := filepath.Join(t.TempDir(), "stat")
	os.WriteFile(f, []byte("9 (my (odd) name) S 1 9 9 0 -1 4194304 1 0 0 0 30 12 0 0 20 0 1 0 5 1 1 18446744073709551615\n"), 0o644)
	s.SelfStat = f
	ticks, err := SelfTicks(s)
	if err != nil {
		t.Fatal(err)
	}
	approx(t, "ticks", ticks, 42)
}

func TestSelfPercent(t *testing.T) {
	// 50 ticks of 100 per second over 2 s is a quarter of one core.
	got, err := SelfPercent(200, 250, 2*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	approx(t, "percent", got, 25)
	if _, err := SelfPercent(250, 200, time.Second); !errors.Is(err, ErrUnparseable) {
		t.Errorf("backwards counter: err = %v", err)
	}
	if _, err := SelfPercent(1, 2, 0); !errors.Is(err, ErrUnparseable) {
		t.Errorf("zero interval: err = %v", err)
	}
}

func TestSelfErrors(t *testing.T) {
	s := fixtureSources(t)
	s.SelfStat = filepath.Join(t.TempDir(), "gone")
	s.SelfStatus = s.SelfStat
	if _, err := SelfTicks(s); !errors.Is(err, ErrAbsent) {
		t.Errorf("ticks: err = %v, want ErrAbsent", err)
	}
	f := filepath.Join(t.TempDir(), "x")
	os.WriteFile(f, []byte("garbage\n"), 0o644)
	s.SelfStat, s.SelfStatus = f, f
	if _, err := SelfTicks(s); !errors.Is(err, ErrUnparseable) {
		t.Errorf("ticks garbage: err = %v", err)
	}
	if _, err := SelfRSS(s); !errors.Is(err, ErrUnparseable) {
		t.Errorf("rss garbage: err = %v", err)
	}
}
