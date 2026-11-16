// Package recorder writes recordings (DR-0003): one JSONL file per run, a
// meta line first, a meta_end line last whatever ended the run, each line
// written as soon as it is complete, and a free-space guard. The Controller
// is shared by `rptop record` and, later, the dashboard's record keys.
package recorder

import (
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"sync"
	"syscall"
	"time"

	"github.com/rsdoiel/rptop/internal/collector"
	"github.com/rsdoiel/rptop/internal/exitcode"
	"github.com/rsdoiel/rptop/internal/sample"
)

// DefaultMinFree is the free-space guard (DR-0003 8).
const DefaultMinFree = 64 << 20

// DefaultSyncEvery is the longest the file goes without a sync (DR-0003 8).
const DefaultSyncEvery = 10 * time.Second

// ErrLowSpace is returned when free space is below the guard. It is an I/O
// failure (exit 74, DR-0002 9).
var ErrLowSpace = fmt.Errorf("free space is below the guard: %w", exitcode.ErrIO)

/** File is what a recording is written to: an *os.File, or a fake in tests.
 *
 * Example:
 *   var f recorder.File = os.Stdout
 */
type File interface {
	io.Writer
	Sync() error
	Close() error
}

/** Options tunes a Controller. The zero value of every field means the
 * default.
 *
 * Fields:
 *   Dir (string)       — where default-named files go; "" means the current directory
 *   MinFree (uint64)   — free-space guard in bytes; 0 means DefaultMinFree
 *   SyncEvery (time.Duration) — longest between syncs; 0 means DefaultSyncEvery
 *   Now (func() time.Time)    — the clock; nil means time.Now
 *   Free (func(dir string) (uint64, error)) — free bytes on dir's file system; nil means statfs
 *   Create (func(path string) (File, error)) — creates the file exclusively; nil means O_EXCL
 *
 * Example:
 *   c := recorder.NewController(meta, recorder.Options{Dir: "runs"})
 */
type Options struct {
	Dir       string
	MinFree   uint64
	SyncEvery time.Duration
	Now       func() time.Time
	Free      func(dir string) (uint64, error)
	Create    func(path string) (File, error)
}

/** Status is a snapshot of the controller.
 *
 * Fields:
 *   Recording (bool)          — a run is open
 *   Path, Run, Label (string) — the file, the run id and the label
 *   Samples (int)             — samples written so far
 *   Started (time.Time)       — when the run began
 *   Reason (string)           — why the last run ended; "" while recording
 *
 * Example:
 *   if c.Status().Recording { ... }
 */
type Status struct {
	Recording  bool
	Path       string
	Run, Label string
	Samples    int
	Started    time.Time
	Reason     string
}

/** Controller starts, feeds and stops recordings. It is safe for use from
 * several goroutines: the sampler's goroutine feeds it while a key handler
 * starts and stops.
 *
 * Example:
 *   st, err := c.Start("before", "")
 */
type Controller struct {
	meta func(time.Time) sample.Meta
	o    Options

	mu       sync.Mutex
	st       Status
	f        File
	lastSync time.Time
	lastSeq  int
}

/** NewController returns an idle Controller.
 *
 * Parameters:
 *   meta (func(time.Time) sample.Meta) — builds the meta line for a run starting at the given time
 *   o    (Options)                     — tuning; the zero value is fine
 *
 * Returns:
 *   *Controller — idle
 *
 * Example:
 *   c := recorder.NewController(smp.Meta, recorder.Options{})
 */
func NewController(meta func(time.Time) sample.Meta, o Options) *Controller {
	if o.MinFree == 0 {
		o.MinFree = DefaultMinFree
	}
	if o.SyncEvery == 0 {
		o.SyncEvery = DefaultSyncEvery
	}
	if o.Now == nil {
		o.Now = time.Now
	}
	if o.Free == nil {
		o.Free = freeBytes
	}
	if o.Create == nil {
		o.Create = createExclusive
	}
	return &Controller{meta: meta, o: o}
}

// freeBytes is the space available to an unprivileged process on dir's file system.
func freeBytes(dir string) (uint64, error) {
	var s syscall.Statfs_t
	if err := syscall.Statfs(dir, &s); err != nil {
		return 0, err
	}
	return uint64(s.Bavail) * uint64(s.Bsize), nil
}

// createExclusive creates path and fails if it exists: a recording is never
// appended to or overwritten.
func createExclusive(path string) (File, error) {
	return os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o644)
}

/** Start begins a run: it picks the file, checks the space, creates the file
 * exclusively and writes and flushes the meta line.
 *
 * Parameters:
 *   label (string) — the run label, "" for none
 *   path  (string) — "" for rptop-YYYYMMDD-HHMMSS.jsonl (UTC) in Options.Dir;
 *                    a directory (existing, or ending in "/") for that name inside it, made
 *                    if needed; otherwise a file, whose directory is made if needed
 *
 * Returns:
 *   Status — the new run
 *   error  — exit 73 if the file exists or its directory cannot be made, 74 if
 *            space is below the guard or the meta line cannot be written, 77 if
 *            access was refused, 2 if a run is already open
 *
 * Example:
 *   st, err := c.Start("before-ollama", "runs/")
 */
func (c *Controller) Start(label, path string) (Status, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.st.Recording {
		return c.st, fmt.Errorf("a recording is already open (%s): %w", c.st.Path, exitcode.ErrUsage)
	}
	now := c.o.Now()
	id := now.UTC().Format("20060102-150405")
	name := "rptop-" + id + ".jsonl"
	switch {
	case path == "":
		path = filepath.Join(c.o.Dir, name)
	case os.IsPathSeparator(path[len(path)-1]): // "runs/": a directory, made below if need be
		path = filepath.Join(path, name)
	default:
		if fi, err := os.Stat(path); err == nil && fi.IsDir() {
			path = filepath.Join(path, name)
		}
	}
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return Status{}, fmt.Errorf("%s: %w: %v", dir, exitcode.ErrCantCreate, err)
	}
	free, err := c.o.Free(dir)
	if err != nil {
		return Status{}, fmt.Errorf("%s: %w: %v", dir, exitcode.ErrIO, err)
	}
	if free < c.o.MinFree {
		return Status{}, fmt.Errorf("%s: %w (%d MiB free)", dir, ErrLowSpace, free>>20)
	}
	f, err := c.o.Create(path)
	if err != nil {
		switch {
		case errors.Is(err, fs.ErrExist):
			return Status{}, fmt.Errorf("%s: file exists: %w", path, exitcode.ErrCantCreate)
		case errors.Is(err, fs.ErrPermission):
			return Status{}, fmt.Errorf("%s: %w", path, collector.ErrRefused)
		}
		return Status{}, fmt.Errorf("%s: %w: %v", path, exitcode.ErrCantCreate, err)
	}
	m := c.meta(now)
	m.Run, m.Label, m.TS = id, label, now
	if _, err := f.Write(append(m.JSON(), '\n')); err != nil {
		f.Close()
		return Status{}, fmt.Errorf("%s: %w: %v", path, exitcode.ErrIO, err)
	}
	f.Sync()
	c.f, c.lastSync, c.lastSeq = f, now, -1
	c.st = Status{Recording: true, Path: path, Run: id, Label: label, Started: now}
	return c.st, nil
}

/** Feed writes one sample to the open run, if there is one. It sets the run,
 * the label and the sequence number itself, so a run that starts while the
 * sampler is already going still counts from 0 without gaps. Before writing it
 * checks the free space; below the guard it ends the run with low_space.
 *
 * Parameters:
 *   s (sample.Sample) — the sampler's sample
 *
 * Returns:
 *   error — nil when idle or written; ErrLowSpace (74) or a write failure (74),
 *           after which the run has ended with a meta_end line
 *
 * Example:
 *   err := c.Feed(s)
 */
func (c *Controller) Feed(s sample.Sample) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if !c.st.Recording {
		return nil
	}
	free, err := c.o.Free(filepath.Dir(c.st.Path))
	if err == nil && free < c.o.MinFree {
		c.end(sample.ReasonLowSpace)
		return fmt.Errorf("%s: %w (%d MiB free)", c.st.Path, ErrLowSpace, free>>20)
	}
	s.Run, s.Label, s.Seq = c.st.Run, c.st.Label, c.st.Samples
	if _, err := c.f.Write(append(s.JSON(), '\n')); err != nil {
		c.end(sample.ReasonWriteError)
		return fmt.Errorf("%s: %w: %v", c.st.Path, exitcode.ErrIO, err)
	}
	c.st.Samples++
	c.lastSeq = s.Seq
	if now := c.o.Now(); now.Sub(c.lastSync) >= c.o.SyncEvery {
		c.f.Sync()
		c.lastSync = now
	}
	return nil
}

/** Marker writes a marker line at the current position of the open run.
 *
 * Parameters:
 *   text (string) — what the marker says
 *
 * Returns:
 *   error — a usage error when no run is open; a write failure (74), after
 *           which the run has ended
 *
 * Example:
 *   err := c.Marker("before benchmark")
 */
func (c *Controller) Marker(text string) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if !c.st.Recording {
		return fmt.Errorf("no recording is open: %w", exitcode.ErrUsage)
	}
	m := sample.Marker{Run: c.st.Run, Label: c.st.Label, TS: c.o.Now(), Seq: c.lastSeq, Text: text}
	if _, err := c.f.Write(append(m.JSON(), '\n')); err != nil {
		c.end(sample.ReasonWriteError)
		return fmt.Errorf("%s: %w: %v", c.st.Path, exitcode.ErrIO, err)
	}
	return nil
}

/** Stop ends the open run with a meta_end line and closes the file. Stopping
 * when nothing is open does nothing.
 *
 * Parameters:
 *   reason (string) — sample.ReasonInterrupt, ReasonDuration or ReasonStopped
 *
 * Returns:
 *   Status — the finished run (Recording false, Reason set)
 *   error  — a write failure (74) while writing meta_end
 *
 * Example:
 *   st, err := c.Stop(sample.ReasonInterrupt)
 */
func (c *Controller) Stop(reason string) (Status, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if !c.st.Recording {
		return c.st, nil
	}
	err := c.end(reason)
	return c.st, err
}

/** Status returns a snapshot.
 *
 * Returns:
 *   Status — the open run, or the last one that ended
 *
 * Example:
 *   fmt.Println(c.Status().Samples)
 */
func (c *Controller) Status() Status {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.st
}

// end writes meta_end, syncs and closes. The caller holds c.mu.
func (c *Controller) end(reason string) error {
	now := c.o.Now()
	e := sample.MetaEnd{Run: c.st.Run, Label: c.st.Label, TS: now, Samples: c.st.Samples,
		DurationMS: now.Sub(c.st.Started).Milliseconds(), Reason: reason}
	var err error
	if _, werr := c.f.Write(append(e.JSON(), '\n')); werr != nil {
		err = fmt.Errorf("%s: meta_end: %w: %v", c.st.Path, exitcode.ErrIO, werr)
	}
	c.f.Sync()
	c.f.Close()
	c.f = nil
	c.st.Recording, c.st.Reason = false, reason
	return err
}
