package cli

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"strconv"
	"strings"
	"time"

	"github.com/rsdoiel/rptop/internal/collector"
	"github.com/rsdoiel/rptop/internal/reading"
)

// usageErr prints a usage error for a verb and returns ExitUsage.
func usageErr(stderr io.Writer, verb string, err error) int {
	fmt.Fprintf(stderr, "%s: %s: %v. Run '%s help %s' for usage.\n", appName, verb, err, appName, verb)
	return ExitUsage
}

// parseInterleaved parses fs over args, allowing options after the
// positional arguments (`rptop temp soc -above 70`).
func parseInterleaved(fs *flag.FlagSet, args []string) ([]string, error) {
	var pos []string
	for {
		if err := fs.Parse(args); err != nil {
			return nil, err
		}
		if fs.NArg() == 0 {
			return pos, nil
		}
		pos = append(pos, fs.Arg(0))
		args = fs.Args()[1:]
	}
}

// newVerbFlags builds the FlagSet shared by every verb: help is always there.
func newVerbFlags(verb string) (*flag.FlagSet, *standardOptions) {
	fs := flag.NewFlagSet(appName+" "+verb, flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	fs.Usage = func() {}
	var std standardOptions
	std.register(fs, false)
	return fs, &std
}

// errorObject is the JSON form of a failure on standard output (DR-0011).
type errorObject struct {
	Error string `json:"error"`
	Class string `json:"class"`
	Code  int    `json:"code"`
	Name  string `json:"name,omitempty"`
}

// reportError writes one failure: plain text on stderr and, in JSON mode, an
// error object on stdout. It returns the exit code of the error.
func reportError(stdout, stderr io.Writer, asJSON bool, name, label string, err error) int {
	code := ExitCode(err)
	fmt.Fprintf(stderr, "%s: %s: %v\n", appName, label, err)
	if asJSON {
		b, _ := json.Marshal(errorObject{Error: err.Error(), Class: ClassName(code), Code: code, Name: name})
		fmt.Fprintf(stdout, "%s\n", b)
	}
	return code
}

/** runVerb runs a reading verb or throttle or list.
 *
 * Parameters:
 *   env    (Env)       — the machine
 *   verb   (string)    — one of the verbs with a help page
 *   args   ([]string)  — the arguments after the verb
 *   stdout (io.Writer) — readings
 *   stderr (io.Writer) — errors, warnings and counts
 *
 * Returns:
 *   int — the exit code, per DR-0002 9
 *
 * Example:
 *   code := runVerb(env, "temp", []string{"soc", "-above", "70"}, os.Stdout, os.Stderr)
 */
func runVerb(env Env, verb string, args []string, stdout, stderr io.Writer) int {
	switch verb {
	case "throttle":
		return runThrottle(env, args, stdout, stderr)
	case "list":
		return runList(env, args, stdout, stderr)
	}
	fs, std := newVerbFlags(verb)
	var number, asJSON bool
	fs.BoolVar(&number, "n", false, "print the bare number")
	fs.BoolVar(&number, "number", false, "print the bare number")
	fs.BoolVar(&asJSON, "j", false, "print JSON")
	fs.BoolVar(&asJSON, "json", false, "print JSON")
	var above, below float64
	var hasAbove, hasBelow bool
	setNum := func(dst *float64, has *bool) func(string) error {
		return func(s string) error {
			v, err := strconv.ParseFloat(s, 64)
			if err != nil {
				return fmt.Errorf("not a number: %q", s)
			}
			*dst, *has = v, true
			return nil
		}
	}
	fs.Func("above", "exit 1 unless the reading is above X", setNum(&above, &hasAbove))
	fs.Func("below", "exit 1 unless the reading is below X", setNum(&below, &hasBelow))
	interval := time.Second
	if rateVerbs[verb] {
		fs.DurationVar(&interval, "interval", time.Second, "time between the two samples")
		fs.DurationVar(&interval, "i", time.Second, "time between the two samples")
	}
	pos, err := parseInterleaved(fs, args)
	if err != nil {
		return usageErr(stderr, verb, err)
	}
	if std.help {
		helpPage(stdout, verbHelp[verb])
		return ExitOK
	}
	if number && asJSON {
		return usageErr(stderr, verb, errors.New("-n and -j cannot be used together"))
	}
	if interval <= 0 {
		return usageErr(stderr, verb, errors.New("-interval must be positive"))
	}

	c := &ctx{env: env, interval: interval}
	defer c.closeAll()
	all := elementsOf(verb, c)

	var selected []elem
	group := len(pos) == 0
	switch {
	case group:
		selected = all
	case verb == "cpu" && pos[0] == "core":
		if len(pos) != 2 {
			return usageErr(stderr, verb, errors.New("core needs one number: cpu core N"))
		}
		n, err := strconv.Atoi(pos[1])
		if err != nil || n < 0 {
			return usageErr(stderr, verb, fmt.Errorf("core number must be 0 or more, got %q", pos[1]))
		}
		selected = []elem{coreElem(n)}
	case len(pos) > 1:
		return usageErr(stderr, verb, fmt.Errorf("surplus argument %q", pos[1]))
	default:
		for _, e := range all {
			if e.short == pos[0] {
				selected = []elem{e}
			}
		}
		if selected == nil {
			return usageErr(stderr, verb, fmt.Errorf("unknown element %q", pos[0]))
		}
	}

	var failed, read int
	firstCode := ExitOK
	negative := false
	for _, e := range selected {
		name := verb + "." + e.short
		label := verb + " " + e.short
		v, err := e.read(c)
		if err != nil {
			failed++
			code := reportError(stdout, stderr, asJSON, name, label, err)
			if firstCode == ExitOK {
				firstCode = code
			}
			continue
		}
		read++
		r := reading.Reading{Name: name, Value: v, Unit: e.unit, Digits: e.digits}
		switch {
		case asJSON:
			fmt.Fprintln(stdout, r.JSON())
		case number:
			fmt.Fprintln(stdout, r.Number())
		case group:
			fmt.Fprintf(stdout, "%s %s\n", e.short, r.String())
		default:
			fmt.Fprintln(stdout, r.String())
		}
		if hasAbove && !(v > above) || hasBelow && !(v < below) {
			negative = true
		}
	}
	if group && failed > 0 {
		fmt.Fprintf(stderr, "%s: %s: %d read, %d failed\n", appName, verb, read, failed)
	}
	if firstCode != ExitOK {
		return firstCode
	}
	if negative {
		return ExitNegative
	}
	return ExitOK
}

func runThrottle(env Env, args []string, stdout, stderr io.Writer) int {
	const verb = "throttle"
	fs, std := newVerbFlags(verb)
	var asJSON, sinceBoot bool
	fs.BoolVar(&asJSON, "j", false, "print JSON")
	fs.BoolVar(&asJSON, "json", false, "print JSON")
	fs.BoolVar(&sinceBoot, "since-boot", false, "let the history decide the exit code")
	pos, err := parseInterleaved(fs, args)
	if err != nil {
		return usageErr(stderr, verb, err)
	}
	if std.help {
		helpPage(stdout, verbHelp[verb])
		return ExitOK
	}
	if len(pos) > 0 {
		return usageErr(stderr, verb, fmt.Errorf("surplus argument %q", pos[0]))
	}
	th, err := collector.Throttled(env.Sources)
	if err != nil {
		return reportError(stdout, stderr, asJSON, verb, verb, err)
	}
	now, since := th.Now(), th.SinceBoot()
	if asJSON {
		b, _ := json.Marshal(struct {
			Name      string   `json:"name"`
			Raw       string   `json:"raw"`
			Now       []string `json:"now"`
			SinceBoot []string `json:"since_boot"`
		}{verb, fmt.Sprintf("%#x", th.Raw), now, since})
		fmt.Fprintf(stdout, "%s\n", b)
	} else {
		list := func(f []string) string {
			if len(f) == 0 {
				return "none"
			}
			return strings.Join(f, ", ")
		}
		fmt.Fprintf(stdout, "now: %s\nsince boot: %s\n", list(now), list(since))
	}
	if (sinceBoot && len(since) > 0) || (!sinceBoot && len(now) > 0) {
		return ExitNegative
	}
	return ExitOK
}

func runList(env Env, args []string, stdout, stderr io.Writer) int {
	const verb = "list"
	fs, std := newVerbFlags(verb)
	var asJSON bool
	fs.BoolVar(&asJSON, "j", false, "print JSON")
	fs.BoolVar(&asJSON, "json", false, "print JSON")
	pos, err := parseInterleaved(fs, args)
	if err != nil {
		return usageErr(stderr, verb, err)
	}
	if std.help {
		helpPage(stdout, verbHelp[verb])
		return ExitOK
	}
	if len(pos) > 0 {
		return usageErr(stderr, verb, fmt.Errorf("surplus argument %q", pos[0]))
	}
	c := &ctx{env: env, interval: time.Second}
	defer c.closeAll()
	type row struct {
		name, unit string
		err        error
	}
	var rows []row
	for _, v := range verbOrder {
		for _, e := range elementsOf(v, c) {
			var err error
			if e.probe != nil {
				err = e.probe(c)
			} else {
				_, err = e.read(c)
			}
			rows = append(rows, row{v + "." + e.short, e.unit, err})
		}
	}
	_, terr := collector.Throttled(env.Sources)
	rows = append(rows, row{"throttle", "", terr})
	for _, r := range rows {
		status := "available"
		class := ""
		if r.err != nil {
			class = ClassName(ExitCode(r.err))
			status = class
		}
		if asJSON {
			b, _ := json.Marshal(struct {
				Name      string `json:"name"`
				Unit      string `json:"unit"`
				Available bool   `json:"available"`
				Class     string `json:"class,omitempty"`
			}{r.name, r.unit, r.err == nil, class})
			fmt.Fprintf(stdout, "%s\n", b)
			continue
		}
		unit := r.unit
		if unit == "" {
			unit = "-"
		}
		fmt.Fprintf(stdout, "%-12s %-6s %s\n", r.name, unit, status)
	}
	return ExitOK
}
