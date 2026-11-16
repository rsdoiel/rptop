package cli

import "strings"

// verbDoc describes one verb for its help page.
type verbDoc struct {
	name       string
	synopsis   string
	summary    string
	elements   [][2]string // element, description
	options    [][2]string // extra options beyond -n, -j, -above, -below
	notes      string
	examples   string
	readonly   bool   // false for throttle, list and record, which have their own options
	exitStatus string // replaces verbExitStatus when set
}

const verbExitStatus = `# EXIT STATUS

0
: the reading was printed and every given condition holds

1
: the reading was printed and -above or -below does not hold, or throttle reports something active

2
: the command line is wrong: unknown element or option, surplus or missing argument, a value that is not a number; nothing was read

65
: a sensor's text could not be parsed

66
: the element is not present on this machine (no such sensor, no vcgencmd, built without the hailo tag, no Hailo device)

69
: the element is present but cannot be reached (the Hailo device cannot be opened or read)

70
: internal error

77
: the operating system refused access to a sensor

With no element, every element of the group is reported. Elements that cannot be read
are reported on standard error and the rest are still printed; the exit code is the
class of the first failure in the order shown above, and the counts are printed.
`

var verbDocs = []verbDoc{
	{name: "temp", readonly: true,
		synopsis: "{app_name} temp [soc|pmic|hailo|rp1] [-n] [-j] [-above X] [-below X]",
		summary:  "Temperatures in degrees Celsius.",
		elements: [][2]string{{"soc", "the SoC (thermal zone 0)"}, {"pmic", "the power-management IC (vcgencmd)"},
			{"hailo", "the Hailo chip, the higher of its two sensors (needs the hailo build tag and a device)"},
			{"rp1", "the RP1 chip (hwmon rp1_adc)"}},
		examples: "{app_name} temp soc -above 70 && echo hot"},
	{name: "cpu", readonly: true,
		synopsis: "{app_name} cpu [total|core N|freq|load] [-interval D] [-n] [-j] [-above X] [-below X]",
		summary:  "CPU utilisation in percent, frequency in MHz and the one-minute load average.",
		elements: [][2]string{{"total", "utilisation of all cores, in %"}, {"core N", "utilisation of core N, in % (two arguments: cpu core 2; named core2 in output)"},
			{"freq", "current frequency in MHz"}, {"load", "one-minute load average"}},
		options:  [][2]string{{"-i, -interval D", "time between the two samples utilisation needs, a Go duration; default 1s"}},
		notes:    "Utilisation is the difference between two samples of /proc/stat, so it takes one interval.",
		examples: "{app_name} cpu core 2\n\n{app_name} cpu total -i 500ms -n"},
	{name: "mem", readonly: true,
		synopsis: "{app_name} mem [used|total|swap] [-n] [-j] [-above X] [-below X]",
		summary:  "Memory in bytes.",
		elements: [][2]string{{"used", "total minus available"}, {"total", "installed memory"}, {"swap", "swap in use"}},
		examples: "{app_name} mem used -n"},
	{name: "fan", readonly: true,
		synopsis: "{app_name} fan [speed] [-n] [-j] [-above X] [-below X]",
		summary:  "The fan speed in revolutions per minute.",
		elements: [][2]string{{"speed", "the fan tachometer (hwmon pwmfan)"}},
		examples: "{app_name} fan -below 1000 || echo fan stalled"},
	{name: "volts", readonly: true,
		synopsis: "{app_name} volts [core] [-n] [-j] [-above X] [-below X]",
		summary:  "The core voltage in volts.",
		elements: [][2]string{{"core", "the core voltage (vcgencmd)"}},
		examples: "{app_name} volts core"},
	{name: "disk", readonly: true,
		synopsis: "{app_name} disk [read|write] [-interval D] [-n] [-j] [-above X] [-below X]",
		summary:  "Disk throughput in bytes per second, summed over the whole physical disks.",
		elements: [][2]string{{"read", "bytes read per second"}, {"write", "bytes written per second"}},
		options:  [][2]string{{"-i, -interval D", "time between the two samples, a Go duration; default 1s"}},
		notes:    "Partitions, ram, loop and zram devices are not counted. Throughput is the difference between two samples of /proc/diskstats.",
		examples: "{app_name} disk write -i 5s"},
	{name: "net", readonly: true,
		synopsis: "{app_name} net [rx|tx] [-interval D] [-n] [-j] [-above X] [-below X]",
		summary:  "Network throughput in bytes per second, summed over every interface except loopback.",
		elements: [][2]string{{"rx", "bytes received per second"}, {"tx", "bytes sent per second"}},
		options:  [][2]string{{"-i, -interval D", "time between the two samples, a Go duration; default 1s"}},
		notes:    "Throughput is the difference between two samples of /proc/net/dev.",
		examples: "{app_name} net rx -n"},
	{name: "throttle",
		synopsis: "{app_name} throttle [-since-boot] [-j]",
		summary:  "What the firmware is throttling now and what has happened since boot.",
		elements: [][2]string{{"under-voltage", "supply voltage dropped"}, {"arm frequency capped", "the ARM clock was capped"},
			{"throttled", "the CPU was throttled"}, {"soft temperature limit", "the soft temperature limit was reached"}},
		options: [][2]string{{"-since-boot", "let the history decide the exit code instead of the present"},
			{"-j, -json", `print {"name","raw","now":[],"since_boot":[]}`}},
		notes:    "Prints two lines, now: and since boot:, each listing the flags or none. Exits 0 when nothing is active now and 1 when something is, so a script is not tripped by an event hours ago. There is no -n, -above or -below.",
		examples: "{app_name} throttle || echo throttled right now"},
	{name: "record",
		synopsis: "{app_name} record [-label TEXT] [-out PATH] [-interval D] [-duration D]",
		summary:  "Record every element to a JSONL file until interrupted, in the foreground.",
		options: [][2]string{
			{"-label TEXT", "a name for the run, stored on every line; use it to compare runs"},
			{"-out PATH", "a file, or an existing directory; the default is rptop-YYYYMMDD-HHMMSS.jsonl (UTC) in the current directory. A directory that does not exist is made. An existing file is never appended to or overwritten"},
			{"-i, -interval D", "time between samples, a Go duration; default 1s, at least 500ms"},
			{"-duration D", "stop after this long; default until Ctrl-C or SIGTERM"}},
		notes: "Each run is one file: a meta line, one sample line per interval, and a meta_end line that is the last line whatever ended the run. " +
			"A file with no meta_end was cut short by a crash or power loss. Keys are the element names of rptop list, a reading that cannot be taken is null, " +
			"and the units are in the meta line. The first sample is written one interval after the start. " +
			"If free space falls below 64 MiB the run ends with reason low_space and the command exits 74. " +
			"The path of the finished file is printed on standard output; progress goes to standard error. See DR-0003 for the format.",
		examples: "{app_name} record -label before-ollama -duration 10m\n\nf=$({app_name} record -duration 60s -out runs/) && jq -s 'map(.[\"temp.soc\"]) | max' \"$f\"",
		exitStatus: `# EXIT STATUS

0
: the run ended on Ctrl-C, SIGTERM or -duration, and meta_end was written

2
: the command line is wrong: -interval below 500ms or not positive, a negative -duration, a surplus argument

73
: the file already exists, or its directory cannot be made

74
: a write failed, or free space fell below the guard; the run ended with meta_end where it could

77
: the operating system refused access to the output
`},
	{name: "list",
		synopsis: "{app_name} list [-j]",
		summary:  "Every element, its unit and whether it can be read on this machine.",
		options:  [][2]string{{"-j, -json", "print one JSON object per element"}},
		notes:    "The last column is available, or the class of the failure (no_input, unavailable, data, no_permission). list exits 0 whatever it finds.",
		examples: "{app_name} list"},
}

// buildVerbHelp renders every verbDoc as a man page source.
func buildVerbHelp() map[string]string {
	out := map[string]string{}
	for _, d := range verbDocs {
		var b strings.Builder
		b.WriteString("%{app_name}-" + d.name + "(1) user manual | version {version} {release_hash}\n% R. S. Doiel\n% {release_date}\n\n")
		b.WriteString("# NAME\n\n{app_name}-" + d.name + " — " + strings.ToLower(d.summary[:1]) + d.summary[1:] + "\n\n")
		b.WriteString("# SYNOPSIS\n\n" + d.synopsis + "\n\n")
		b.WriteString("# DESCRIPTION\n\n" + d.summary + "\n\n")
		if len(d.elements) > 0 {
			b.WriteString("## Elements\n\n")
			for _, e := range d.elements {
				b.WriteString(e[0] + "\n: " + e[1] + "\n\n")
			}
		}
		b.WriteString("# OPTIONS\n\n-h, -help\n: display help\n\n")
		if d.readonly {
			b.WriteString("-n, -number\n: print the bare number, without its unit\n\n" +
				"-j, -json\n: print JSON, for example {\"name\":\"temp.soc\",\"value\":54.6,\"unit\":\"°C\"}; a failure is also an error object with its class and code\n\n" +
				"-above X\n: exit 1 unless the reading is above X; the reading is still printed\n\n" +
				"-below X\n: exit 1 unless the reading is below X; the reading is still printed\n\n")
		}
		for _, o := range d.options {
			b.WriteString(o[0] + "\n: " + o[1] + "\n\n")
		}
		if d.notes != "" {
			b.WriteString(d.notes + "\n\n")
		}
		if d.exitStatus != "" {
			b.WriteString(d.exitStatus)
		} else {
			b.WriteString(verbExitStatus)
		}
		if d.examples != "" {
			b.WriteString("\n# EXAMPLES\n\n~~~shell\n" + d.examples + "\n~~~\n")
		}
		out[d.name] = b.String()
	}
	return out
}

// verbHelp maps each verb to its help page.
var verbHelp = buildVerbHelp()
