package cli

/** HelpText is the primary rptop(1) page, shown by `rptop -help` and
 * `rptop help`.
 *
 * Example:
 *   fmt.Print(rptop.FmtHelp(cli.HelpText, "rptop", rptop.Version, rptop.ReleaseDate, rptop.ReleaseHash))
 */
const HelpText = `%{app_name}(1) user manual | version {version} {release_hash}
% R. S. Doiel
% {release_date}

# NAME

{app_name} — a Raspberry Pi vitals dashboard and scriptable reporter

# SYNOPSIS

{app_name} [-h|-help] [-l|-license] [-v|-version]

{app_name} VERB [ELEMENT] [-n|-number] [-j|-json] [-above X] [-below X]

{app_name} list [-j|-json]

{app_name} help [VERB]

# DESCRIPTION

{app_name} reports the vitals of a Raspberry Pi 5, with an optional AI HAT+ 2:
CPU, memory, temperatures, throttling, fan, disk and network. A dashboard and a
verb-oriented command line read the same collectors, so every element on the
dashboard is also a verb that reports one value and its unit.

# VERBS

temp
: temperatures in °C: soc, pmic, hailo, rp1

cpu
: utilisation in %, frequency in MHz, load average: total, core N, freq, load

mem
: memory in bytes: used, total, swap

fan
: fan speed in rpm

volts
: core voltage in V

disk
: bytes per second read and written

net
: bytes per second received and sent

throttle
: what the firmware is throttling now and since boot

list
: every element, its unit and whether it can be read on this machine

A verb with no element reports every element of its group, one line each.
Run {app_name} help VERB to read about a verb.

# EXIT STATUS

Exit codes follow the workspace convention (DR-0003).

0
: success

1
: the command ran and the answer is no

2
: the command line is wrong; nothing was attempted

65
: content the tool reads is wrong

66
: a named input, or a sensor on this machine, is missing

69
: a service the command needs cannot be reached

70
: internal error

73
: an output cannot be created

74
: a read or write failed part way

75
: retrying later may succeed

77
: the operating system refused access

78
: a configuration file is present but wrong
`
