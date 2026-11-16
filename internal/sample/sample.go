// Package sample defines the lines of an rptop recording (DR-0003): the meta
// line, the samples, markers and the meta_end line, with their JSON encoding
// and a reader for them. The sampler builds them, the recorder writes them
// and the dashboard shows them, so all three agree.
package sample

import (
	"bytes"
	"encoding/json"
	"fmt"
	"math"
	"strconv"
	"time"

	"github.com/rsdoiel/rptop/internal/collector"
)

// Schema is the recording format version; it changes only for an
// incompatible change (DR-0003 4).
const Schema = 1

// Reasons a run ends (DR-0003 7).
const (
	ReasonInterrupt  = "interrupt"
	ReasonDuration   = "duration"
	ReasonStopped    = "stopped"
	ReasonLowSpace   = "low_space"
	ReasonWriteError = "write_error"
)

// ThrottleBits names bits 0 to 3 of throttle.now and throttle.since_boot.
var ThrottleBits = []string{"under-voltage", "arm frequency capped", "throttled", "soft temperature limit"}

/** Entry is one named reading in a sample. A nil Value is a reading that could
 * not be taken this tick and is written as null.
 *
 * Fields:
 *   Name  (string)   — the dotted element name, such as "temp.soc"
 *   Value (*float64) — the reading, or nil
 *
 * Example:
 *   sample.Entry{Name: "temp.soc", Value: sample.Num(54.1)}
 */
type Entry struct {
	Name  string
	Value *float64
}

/** Num returns a pointer to v, for building an Entry.
 *
 * Parameters:
 *   v (float64) — a reading
 *
 * Returns:
 *   *float64 — pointer to a copy of v
 *
 * Example:
 *   e := sample.Entry{Name: "fan.speed", Value: sample.Num(3725)}
 */
func Num(v float64) *float64 { return &v }

/** Sample is one tick: every element's reading at one moment.
 *
 * Fields:
 *   Run, Label (string) — the run id and the optional label, on every line
 *   Seq (int)           — counts from 0 with no gaps
 *   TS (time.Time)      — when the tick fired
 *   DtMS (int64)        — measured milliseconds since the previous tick
 *   Entries ([]Entry)   — readings, in `rptop list` order
 *
 * Example:
 *   line := s.JSON()
 */
type Sample struct {
	Run, Label string
	Seq        int
	TS         time.Time
	DtMS       int64
	Entries    []Entry
}

// ElementInfo describes one element in the meta line.
type ElementInfo struct {
	Name, Unit string
	Available  bool
	Class      string // failure class by name when not available
}

/** Meta is the first line of a recording.
 *
 * Fields:
 *   Run, Label, Rptop, Host, Kernel, Model (string) — identity and machine
 *   TS (time.Time)         — when the run started
 *   IntervalMS (int)       — the requested interval
 *   Elements ([]ElementInfo) — every element, its unit and availability
 *
 * Example:
 *   line := meta.JSON()
 */
type Meta struct {
	Run, Label, Rptop, Host, Kernel, Model string
	TS                                     time.Time
	IntervalMS                             int
	Elements                               []ElementInfo
}

/** Marker is a labelled point in a run, written by the dashboard's m key.
 *
 * Fields:
 *   Run, Label, Text (string) — identity and the marker text
 *   TS (time.Time)            — when it was made
 *   Seq (int)                 — the sequence number of the last sample
 *
 * Example:
 *   line := marker.JSON()
 */
type Marker struct {
	Run, Label, Text string
	TS               time.Time
	Seq              int
}

/** MetaEnd is the last line of every finished recording.
 *
 * Fields:
 *   Run, Label, Reason (string) — identity and why the run ended
 *   TS (time.Time)              — when it ended
 *   Samples (int), DurationMS (int64) — how much was recorded
 *
 * Example:
 *   line := end.JSON()
 */
type MetaEnd struct {
	Run, Label, Reason string
	TS                 time.Time
	Samples            int
	DurationMS         int64
}

/** Clean reports whether the run ended as intended: true for interrupt,
 * duration and stopped; false for low_space and write_error.
 *
 * Returns:
 *   bool — the clean flag of the meta_end line
 *
 * Example:
 *   sample.MetaEnd{Reason: sample.ReasonLowSpace}.Clean() // false
 */
func (e MetaEnd) Clean() bool {
	return e.Reason == ReasonInterrupt || e.Reason == ReasonDuration || e.Reason == ReasonStopped
}

// writer builds one JSON object with a fixed key order.
type writer struct {
	b     bytes.Buffer
	first bool
}

func newWriter() *writer {
	w := &writer{first: true}
	w.b.WriteByte('{')
	return w
}

func (w *writer) key(k string) {
	if !w.first {
		w.b.WriteByte(',')
	}
	w.first = false
	w.b.Write(quote(k))
	w.b.WriteByte(':')
}

func (w *writer) str(k, v string)        { w.key(k); w.b.Write(quote(v)) }
func (w *writer) int(k string, v int64)  { w.key(k); w.b.WriteString(strconv.FormatInt(v, 10)) }
func (w *writer) raw(k string, v string) { w.key(k); w.b.WriteString(v) }
func (w *writer) bool(k string, v bool)  { w.key(k); w.b.WriteString(strconv.FormatBool(v)) }
func (w *writer) done() []byte           { w.b.WriteByte('}'); return w.b.Bytes() }

func (w *writer) label(l string) {
	if l != "" {
		w.str("label", l)
	}
}

func quote(s string) []byte {
	var b bytes.Buffer
	enc := json.NewEncoder(&b)
	enc.SetEscapeHTML(false)
	enc.Encode(s)
	return bytes.TrimRight(b.Bytes(), "\n")
}

func stamp(t time.Time) string { return t.UTC().Format("2006-01-02T15:04:05.000Z") }

// number is the shortest decimal form, never an exponent; null if not finite.
func number(v *float64) string {
	if v == nil || math.IsNaN(*v) || math.IsInf(*v, 0) {
		return "null"
	}
	return strconv.FormatFloat(*v, 'f', -1, 64)
}

/** JSON returns the sample as one line of JSON without the trailing newline.
 * Key order is fixed (type, run, label, seq, ts, dt_ms, then the entries) so
 * recordings diff cleanly.
 *
 * Returns:
 *   []byte — the line
 *
 * Example:
 *   f.Write(append(s.JSON(), '\n'))
 */
func (s Sample) JSON() []byte {
	w := newWriter()
	w.str("type", "sample")
	w.str("run", s.Run)
	w.label(s.Label)
	w.int("seq", int64(s.Seq))
	w.str("ts", stamp(s.TS))
	w.int("dt_ms", s.DtMS)
	for _, e := range s.Entries {
		w.raw(e.Name, number(e.Value))
	}
	return w.done()
}

/** JSON returns the meta line without the trailing newline.
 *
 * Returns:
 *   []byte — the line
 *
 * Example:
 *   f.Write(append(meta.JSON(), '\n'))
 */
func (m Meta) JSON() []byte {
	w := newWriter()
	w.str("type", "meta")
	w.int("schema", Schema)
	w.str("run", m.Run)
	w.label(m.Label)
	w.str("ts", stamp(m.TS))
	w.str("rptop", m.Rptop)
	w.str("host", m.Host)
	w.str("kernel", m.Kernel)
	w.str("model", m.Model)
	w.int("interval_ms", int64(m.IntervalMS))
	var el bytes.Buffer
	el.WriteByte('{')
	for i, e := range m.Elements {
		if i > 0 {
			el.WriteByte(',')
		}
		el.Write(quote(e.Name))
		el.WriteString(`:{"unit":`)
		el.Write(quote(e.Unit))
		el.WriteString(`,"available":` + strconv.FormatBool(e.Available))
		if e.Class != "" {
			el.WriteString(`,"class":`)
			el.Write(quote(e.Class))
		}
		el.WriteByte('}')
	}
	el.WriteByte('}')
	w.raw("elements", el.String())
	bits, _ := json.Marshal(ThrottleBits)
	w.raw("throttle_bits", string(bits))
	return w.done()
}

/** JSON returns the marker line without the trailing newline.
 *
 * Returns:
 *   []byte — the line
 *
 * Example:
 *   f.Write(append(mk.JSON(), '\n'))
 */
func (m Marker) JSON() []byte {
	w := newWriter()
	w.str("type", "marker")
	w.str("run", m.Run)
	w.label(m.Label)
	w.str("ts", stamp(m.TS))
	w.int("seq", int64(m.Seq))
	w.str("text", m.Text)
	return w.done()
}

/** JSON returns the meta_end line without the trailing newline.
 *
 * Returns:
 *   []byte — the line
 *
 * Example:
 *   f.Write(append(end.JSON(), '\n'))
 */
func (e MetaEnd) JSON() []byte {
	w := newWriter()
	w.str("type", "meta_end")
	w.str("run", e.Run)
	w.label(e.Label)
	w.str("ts", stamp(e.TS))
	w.int("samples", int64(e.Samples))
	w.int("duration_ms", e.DurationMS)
	w.str("reason", e.Reason)
	w.bool("clean", e.Clean())
	return w.done()
}

/** Line is a recording line read back: its type and its keys.
 *
 * Fields:
 *   Type (string)                       — "meta", "sample", "marker", "meta_end" or one this version does not know
 *   Fields (map[string]json.RawMessage) — every key of the line
 *
 * Example:
 *   l, err := sample.Parse(scanner.Bytes())
 */
type Line struct {
	Type   string
	Fields map[string]json.RawMessage
}

/** Parse reads one recording line. A line type this version does not know is
 * accepted, as DR-0003 2 asks.
 *
 * Parameters:
 *   b ([]byte) — one line, without the newline
 *
 * Returns:
 *   Line  — the type and the keys
 *   error — collector.ErrUnparseable (a data error, exit 65) if the line is
 *           not a JSON object with a string "type"
 *
 * Example:
 *   l, err := sample.Parse([]byte(`{"type":"sample","temp.soc":54}`))
 */
func Parse(b []byte) (Line, error) {
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(b, &fields); err != nil {
		return Line{}, fmt.Errorf("recording line: %w: %v", collector.ErrUnparseable, err)
	}
	var typ string
	if err := json.Unmarshal(fields["type"], &typ); err != nil || typ == "" {
		return Line{}, fmt.Errorf("recording line: %w: no string \"type\"", collector.ErrUnparseable)
	}
	return Line{Type: typ, Fields: fields}, nil
}

/** Number returns a key's value as a number.
 *
 * Parameters:
 *   key (string) — a key of the line
 *
 * Returns:
 *   float64 — the value
 *   bool    — false if the key is absent, null or not a number
 *
 * Example:
 *   v, ok := l.Number("temp.soc")
 */
func (l Line) Number(key string) (float64, bool) {
	raw, ok := l.Fields[key]
	if !ok || string(raw) == "null" {
		return 0, false
	}
	var v float64
	if err := json.Unmarshal(raw, &v); err != nil {
		return 0, false
	}
	return v, true
}
