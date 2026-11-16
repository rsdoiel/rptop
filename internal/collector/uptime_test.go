package collector

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestUptime(t *testing.T) {
	s := fixtureSources(t)
	s.Uptime = filepath.Join(filepath.Dir(s.LoadAvg), "proc_uptime")
	got, err := Uptime(s)
	if err != nil {
		t.Fatal(err)
	}
	if want := 123456*time.Second + 780*time.Millisecond; got != want {
		t.Errorf("Uptime = %v, want %v", got, want)
	}
}

func TestUptimeErrors(t *testing.T) {
	s := fixtureSources(t)
	s.Uptime = filepath.Join(t.TempDir(), "gone")
	if _, err := Uptime(s); !errors.Is(err, ErrAbsent) {
		t.Errorf("err = %v, want ErrAbsent", err)
	}
	f := filepath.Join(t.TempDir(), "up")
	os.WriteFile(f, []byte("soon\n"), 0o644)
	s.Uptime = f
	if _, err := Uptime(s); !errors.Is(err, ErrUnparseable) {
		t.Errorf("err = %v, want ErrUnparseable", err)
	}
}
