package sample

import (
	"encoding/json"
	"errors"
	"math"
	"strings"
	"testing"
	"time"

	"github.com/rsdoiel/rptop/internal/collector"
)

var t0 = time.Date(2026, 11, 15, 14, 30, 54, 4_000_000, time.UTC)

func TestSampleGolden(t *testing.T) {
	s := Sample{Run: "20261115-143012", Label: "before", Seq: 42, TS: t0, DtMS: 1003, Entries: []Entry{
		{"temp.soc", Num(54)}, {"temp.hailo", nil}, {"cpu.total", Num(38.2)},
		{"mem.used", Num(2254857830)}, {"volts.core", Num(0.7838)}, {"throttle.now", Num(0)},
	}}
	want := `{"type":"sample","run":"20261115-143012","label":"before","seq":42,"ts":"2026-11-15T14:30:54.004Z","dt_ms":1003,` +
		`"temp.soc":54,"temp.hailo":null,"cpu.total":38.2,"mem.used":2254857830,"volts.core":0.7838,"throttle.now":0}`
	if got := string(s.JSON()); got != want {
		t.Errorf("got  %s\nwant %s", got, want)
	}
	s.Label = ""
	if got := string(s.JSON()); !strings.HasPrefix(got, `{"type":"sample","run":"20261115-143012","seq":42,`) {
		t.Errorf("label key present without a label: %s", got)
	}
}

func TestSampleTimestampIsUTCMilliseconds(t *testing.T) {
	local := time.FixedZone("x", 3600*5)
	s := Sample{Run: "r", TS: time.Date(2026, 11, 15, 19, 30, 54, 4_000_000, local)}
	var m map[string]any
	json.Unmarshal(s.JSON(), &m)
	if m["ts"] != "2026-11-15T14:30:54.004Z" {
		t.Errorf("ts = %v", m["ts"])
	}
}

func TestNonFiniteBecomesNullAndNoExponent(t *testing.T) {
	s := Sample{Run: "r", TS: t0, Entries: []Entry{{"a", Num(math.NaN())}, {"b", Num(math.Inf(1))}, {"c", Num(1e21)}, {"d", Num(0.00001)}}}
	var m map[string]json.RawMessage
	if err := json.Unmarshal(s.JSON(), &m); err != nil {
		t.Fatalf("not valid JSON: %s: %v", s.JSON(), err)
	}
	if string(m["a"]) != "null" || string(m["b"]) != "null" {
		t.Errorf("non-finite values: a=%s b=%s", m["a"], m["b"])
	}
	if string(m["c"]) != "1000000000000000000000" || string(m["d"]) != "0.00001" {
		t.Errorf("c=%s d=%s", m["c"], m["d"])
	}
}

func TestMetaGolden(t *testing.T) {
	m := Meta{Run: "20261115-143012", Label: "before", TS: t0.Add(-43 * time.Second), Rptop: "0.0.0 f17a15b",
		Host: "harvey", Kernel: "6.18.60+rpt-rpi-2712", Model: "Raspberry Pi 5 Model B Rev 1.0", IntervalMS: 1000,
		Elements: []ElementInfo{
			{Name: "temp.soc", Unit: "°C", Available: true},
			{Name: "temp.hailo", Unit: "°C", Available: false, Class: "no_input"},
		}}
	want := `{"type":"meta","schema":1,"run":"20261115-143012","label":"before","ts":"2026-11-15T14:30:11.004Z",` +
		`"rptop":"0.0.0 f17a15b","host":"harvey","kernel":"6.18.60+rpt-rpi-2712","model":"Raspberry Pi 5 Model B Rev 1.0",` +
		`"interval_ms":1000,"elements":{"temp.soc":{"unit":"°C","available":true},"temp.hailo":{"unit":"°C","available":false,"class":"no_input"}},` +
		`"throttle_bits":["under-voltage","arm frequency capped","throttled","soft temperature limit"]}`
	if got := string(m.JSON()); got != want {
		t.Errorf("got  %s\nwant %s", got, want)
	}
}

func TestMetaEndAndMarkerGolden(t *testing.T) {
	e := MetaEnd{Run: "r1", TS: t0, Samples: 300, DurationMS: 300006, Reason: ReasonInterrupt}
	want := `{"type":"meta_end","run":"r1","ts":"2026-11-15T14:30:54.004Z","samples":300,"duration_ms":300006,"reason":"interrupt","clean":true}`
	if got := string(e.JSON()); got != want {
		t.Errorf("got  %s\nwant %s", got, want)
	}
	mk := Marker{Run: "r1", Label: "x", TS: t0, Seq: 7, Text: `before "benchmark"`}
	want = `{"type":"marker","run":"r1","label":"x","ts":"2026-11-15T14:30:54.004Z","seq":7,"text":"before \"benchmark\""}`
	if got := string(mk.JSON()); got != want {
		t.Errorf("got  %s\nwant %s", got, want)
	}
}

// DR-0003 7: clean is true for the first three reasons only.
func TestCleanByReason(t *testing.T) {
	for r, want := range map[string]bool{ReasonInterrupt: true, ReasonDuration: true, ReasonStopped: true, ReasonLowSpace: false, ReasonWriteError: false} {
		if got := (MetaEnd{Reason: r}).Clean(); got != want {
			t.Errorf("Clean(%s) = %v, want %v", r, got, want)
		}
	}
}

func TestParse(t *testing.T) {
	l, err := Parse([]byte((Sample{Run: "r", Seq: 3, TS: t0, Entries: []Entry{{"temp.soc", Num(54)}}}).JSON()))
	if err != nil || l.Type != "sample" {
		t.Fatalf("Parse sample: %v %+v", err, l)
	}
	if v, ok := l.Number("temp.soc"); !ok || v != 54 {
		t.Errorf("Number(temp.soc) = %v %v", v, ok)
	}
	if _, ok := l.Number("nosuch"); ok {
		t.Error("Number of a missing key is ok")
	}
	null, _ := Parse([]byte(`{"type":"sample","temp.hailo":null}`))
	if _, ok := null.Number("temp.hailo"); ok {
		t.Error("null reported as a number")
	}
	if l, err := Parse([]byte(`{"type":"future","x":1}`)); err != nil || l.Type != "future" {
		t.Errorf("unknown type must be accepted: %v %+v", err, l)
	}
}

// DR-0003 2: a line that is not valid JSON is a data error (65).
func TestParseMalformedIsUnparseable(t *testing.T) {
	for _, line := range []string{``, `not json`, `{"type":`, `[1,2]`, `{"run":"r"}`, `{"type":5}`} {
		if _, err := Parse([]byte(line)); !errors.Is(err, collector.ErrUnparseable) {
			t.Errorf("Parse(%q) err = %v, want ErrUnparseable", line, err)
		}
	}
}
