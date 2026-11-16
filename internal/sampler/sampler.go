// Package sampler turns the collectors into a stream of samples (DR-0003).
// Once per interval it reads every element on its own goroutine with its own
// timeout, so one slow or hung collector leaves its own reading null and
// delays neither the tick nor the other readings. The recorder and the
// dashboard consume the same stream, so what is shown is what is recorded.
package sampler

import (
	"context"
	"fmt"
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/rsdoiel/rptop/internal/collector"
	"github.com/rsdoiel/rptop/internal/element"
	"github.com/rsdoiel/rptop/internal/exitcode"
	"github.com/rsdoiel/rptop/internal/sample"
)

// MinInterval is the shortest sampling interval (DR-0003 9).
const MinInterval = 500 * time.Millisecond

/** Config says what to sample and how often.
 *
 * Fields:
 *   Env (element.Env)        — the machine to read
 *   Run, Label (string)      — run id and optional label, on every line
 *   Version (string)         — the rptop version, for the meta line
 *   Interval (time.Duration) — time between samples; at least MinInterval
 *   Timeout (time.Duration)  — how long a tick waits for its readings; 0 means half the interval
 *   Ticks (<-chan time.Time) — drives the sampler in tests; nil means a real ticker
 *   Now (func() time.Time)   — the start time; nil means time.Now
 *
 * Example:
 *   s, err := sampler.New(sampler.Config{Env: element.DefaultEnv(), Run: "20261115-143012", Interval: time.Second})
 */
type Config struct {
	Env      element.Env
	Run      string
	Label    string
	Version  string
	Interval time.Duration
	Timeout  time.Duration
	Ticks    <-chan time.Time
	Now      func() time.Time
}

// job is one read per tick that yields one or more named values.
type job struct {
	names     []string
	units     []string
	minPeriod time.Duration
	read      func(c *element.Ctx) ([]float64, error)
	probe     func(c *element.Ctx) error

	inflight atomic.Bool
	mu       sync.Mutex
	cached   []float64
	cachedAt time.Time
}

/** Sampler produces samples. Create it with New and run it with Run.
 *
 * Example:
 *   err := s.Run(ctx, func(x sample.Sample) { fmt.Println(string(x.JSON())) })
 */
type Sampler struct {
	cfg   Config
	c     *element.Ctx
	jobs  []*job
	ready chan struct{}

	prevSnap  element.Snapshot
	prevT     time.Time
	prevSelf  float64
	haveSelf  bool
	seq       int
	startedAt time.Time
}

/** New checks cfg and prepares a Sampler. Nothing is read until Run or Meta.
 *
 * Parameters:
 *   cfg (Config) — what to sample
 *
 * Returns:
 *   *Sampler — ready to Run
 *   error    — an exitcode.ErrUsage error if Interval is below MinInterval
 *
 * Example:
 *   s, err := sampler.New(cfg)
 */
func New(cfg Config) (*Sampler, error) {
	if cfg.Interval < MinInterval {
		return nil, fmt.Errorf("interval %v is below the %v minimum: %w", cfg.Interval, MinInterval, exitcode.ErrUsage)
	}
	if cfg.Timeout <= 0 {
		cfg.Timeout = cfg.Interval / 2
	}
	if cfg.Now == nil {
		cfg.Now = time.Now
	}
	s := &Sampler{cfg: cfg, c: element.NewCtx(cfg.Env), ready: make(chan struct{})}
	for _, e := range element.All(cfg.Env.Sources) {
		e := e
		s.jobs = append(s.jobs, &job{
			names: []string{e.Name}, units: []string{e.Unit}, minPeriod: e.MinPeriod,
			read: func(c *element.Ctx) ([]float64, error) {
				v, err := e.Read(c)
				return []float64{v}, err
			},
			probe: func(c *element.Ctx) error {
				if e.Probe != nil {
					return e.Probe(c)
				}
				_, err := e.Read(c)
				return err
			},
		})
	}
	// The firmware word, split into its two halves (DR-0003 5).
	th := func(c *element.Ctx) ([]float64, error) {
		t, err := collector.Throttled(c.Env.Sources)
		if err != nil {
			return nil, err
		}
		return []float64{float64(t.Raw & 0xF), float64((t.Raw >> 16) & 0xF)}, nil
	}
	s.jobs = append(s.jobs, &job{
		names: []string{"throttle.now", "throttle.since_boot"}, units: []string{"", ""}, read: th,
		probe: func(c *element.Ctx) error { _, err := th(c); return err },
	})
	return s, nil
}

/** Ready returns a channel closed once Run has taken its baseline. Counters
 * moved before then are not part of the first sample.
 *
 * Returns:
 *   <-chan struct{} — closed when sampling can begin
 *
 * Example:
 *   <-s.Ready()
 */
func (s *Sampler) Ready() <-chan struct{} { return s.ready }

/** Close releases the Hailo device. Run calls it on return; call it yourself
 * only if Meta was used without Run.
 *
 * Example:
 *   defer s.Close()
 */
func (s *Sampler) Close() { s.c.Close() }

// self is rptop's own cost: CPU as percent of one core, and resident memory.
var selfNames = []string{"self.cpu", "self.rss"}
var selfUnits = []string{"%", "bytes"}

func class(err error) string {
	if err == nil {
		return ""
	}
	return exitcode.Class(exitcode.Code(err))
}

func readOne(path string) string {
	b, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	return strings.Trim(strings.TrimSpace(string(b)), "\x00")
}

/** Meta describes the run: the machine, the interval and every key a sample
 * will carry with its unit and whether it could be read now. It probes the
 * sensors, so it may open the Hailo device.
 *
 * Parameters:
 *   now (time.Time) — the start time of the run
 *
 * Returns:
 *   sample.Meta — the first line of the recording
 *
 * Example:
 *   line := s.Meta(time.Now()).JSON()
 */
func (s *Sampler) Meta(now time.Time) sample.Meta {
	host, _ := os.Hostname()
	m := sample.Meta{
		Run: s.cfg.Run, Label: s.cfg.Label, TS: now, Rptop: s.cfg.Version, Host: host,
		Kernel: readOne("/proc/sys/kernel/osrelease"), Model: readOne("/proc/device-tree/model"),
		IntervalMS: int(s.cfg.Interval / time.Millisecond),
	}
	for _, j := range s.jobs {
		err := j.probe(s.c)
		for i, n := range j.names {
			m.Elements = append(m.Elements, sample.ElementInfo{Name: n, Unit: j.units[i], Available: err == nil, Class: class(err)})
		}
	}
	errCPU := func() error { _, err := collector.SelfTicks(s.cfg.Env.Sources); return err }()
	errRSS := func() error { _, err := collector.SelfRSS(s.cfg.Env.Sources); return err }()
	for i, n := range selfNames {
		err := []error{errCPU, errRSS}[i]
		m.Elements = append(m.Elements, sample.ElementInfo{Name: n, Unit: selfUnits[i], Available: err == nil, Class: class(err)})
	}
	return m
}

type result struct {
	j    *job
	vals []float64
	err  error
}

/** Run samples until ctx is cancelled. It takes a baseline, closes Ready, then
 * emits one Sample per tick. The first sample is one interval after the start,
 * so every rate in it has something to compare with. emit is called on the
 * sampling goroutine; a slow emit delays the next tick, so consumers should
 * hand the sample on and return.
 *
 * Parameters:
 *   ctx  (context.Context)    — cancel to stop
 *   emit (func(sample.Sample)) — receives each sample, in order
 *
 * Returns:
 *   error — nil when ctx ends the run
 *
 * Example:
 *   err := s.Run(ctx, func(x sample.Sample) { rec.Write(x) })
 */
func (s *Sampler) Run(ctx context.Context, emit func(sample.Sample)) error {
	defer s.c.Close()
	s.startedAt = s.cfg.Now()
	s.prevT = s.startedAt
	s.prevSnap = element.Take(s.cfg.Env.Sources)
	if v, err := collector.SelfTicks(s.cfg.Env.Sources); err == nil {
		s.prevSelf, s.haveSelf = v, true
	}
	close(s.ready)

	ticks := s.cfg.Ticks
	if ticks == nil {
		tk := time.NewTicker(s.cfg.Interval)
		defer tk.Stop()
		ticks = tk.C
	}
	for {
		select {
		case <-ctx.Done():
			return nil
		case t := <-ticks:
			emit(s.tick(t))
		}
	}
}

// tick builds one sample for the tick that fired at t.
func (s *Sampler) tick(t time.Time) sample.Sample {
	snap := element.Take(s.cfg.Env.Sources)
	dt := t.Sub(s.prevT)
	tc := s.c.WithRates(s.prevSnap, snap, dt)

	results := make(chan result, len(s.jobs))
	pending := 0
	values := map[*job][]float64{}
	for _, j := range s.jobs {
		j.mu.Lock()
		fresh := j.minPeriod > 0 && j.cached != nil && t.Sub(j.cachedAt) < j.minPeriod
		if fresh {
			values[j] = j.cached
		}
		j.mu.Unlock()
		if fresh || !j.inflight.CompareAndSwap(false, true) {
			continue // cached, or the previous read of this job is still running
		}
		pending++
		go func(j *job) {
			defer j.inflight.Store(false)
			vals, err := j.read(tc)
			if err == nil && j.minPeriod > 0 {
				j.mu.Lock()
				j.cached, j.cachedAt = vals, t
				j.mu.Unlock()
			}
			results <- result{j, vals, err}
		}(j)
	}
	timer := time.NewTimer(s.cfg.Timeout)
	defer timer.Stop()
collect:
	for pending > 0 {
		select {
		case r := <-results:
			pending--
			if r.err == nil {
				values[r.j] = r.vals
			}
		case <-timer.C:
			break collect
		}
	}

	out := sample.Sample{Run: s.cfg.Run, Label: s.cfg.Label, Seq: s.seq, TS: t, DtMS: dt.Milliseconds()}
	for _, j := range s.jobs {
		for i, n := range j.names {
			e := sample.Entry{Name: n}
			if v, ok := values[j]; ok && i < len(v) {
				e.Value = sample.Num(v[i])
			}
			out.Entries = append(out.Entries, e)
		}
	}
	cpu := sample.Entry{Name: "self.cpu"}
	if v, err := collector.SelfTicks(s.cfg.Env.Sources); err == nil {
		if s.haveSelf {
			if p, err := collector.SelfPercent(s.prevSelf, v, dt); err == nil {
				cpu.Value = sample.Num(p)
			}
		}
		s.prevSelf, s.haveSelf = v, true
	}
	rss := sample.Entry{Name: "self.rss"}
	if v, err := collector.SelfRSS(s.cfg.Env.Sources); err == nil {
		rss.Value = sample.Num(v)
	}
	out.Entries = append(out.Entries, cpu, rss)

	s.prevSnap, s.prevT = snap, t
	s.seq++
	return out
}
