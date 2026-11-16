%rptop(1) user manual | version 0.0.1 e50a5ea
% R. S. Doiel
% 2026-11-15

# NAME

rptop — a Raspberry Pi vitals dashboard and scriptable reporter

# SYNOPSIS

rptop [-h|-help] [-l|-license] [-v|-version]

rptop [-i|-interval D] [-warn-temp C] [-crit-temp C] [-out PATH]

rptop VERB [ELEMENT] [-n|-number] [-j|-json] [-above X] [-below X]

rptop list [-j|-json]

rptop record [-label TEXT] [-out PATH] [-i|-interval D] [-duration D]

rptop help [VERB]

# DESCRIPTION

rptop reports the vitals of a Raspberry Pi 5, with an optional AI HAT+ 2:
CPU, memory, temperatures, throttling, fan, disk and network. A dashboard and a
verb-oriented command line read the same collectors, so every element on the
dashboard is also a verb that reports one value and its unit.

# DASHBOARD

Run rptop with no verb in a terminal to open the dashboard: CPU and
per-core load, memory, temperatures with sparklines, throttling, fan and
voltage, and disk and network throughput. It needs a terminal at least 72
columns by 20 rows; without one it prints this page to standard error and
exits 2. There is no configuration file; these options are all of it.

-i, -interval D
: time between samples, a Go duration; default 1s, at least 500ms

-warn-temp C
: temperatures at or above this are yellow; default 60

-crit-temp C
: temperatures at or above this are red; default 80, and above -warn-temp

-out PATH
: where recordings started with r go; a directory, made if needed; default the current directory

Keys:

q, ctrl+c
: quit; an open recording is ended first

r
: start a recording, or stop the open one

R
: ask for a label, then start a recording

m
: ask for marker text and write it to the open recording

p
: pause the display; sampling and recording go on

?
: show the keys

A recording from the dashboard is the same JSONL file as rptop record
writes; if one cannot start or has to stop, the dashboard shows a banner and
keeps running.

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

record
: record every element to a JSONL file, one file per run, until Ctrl-C or -duration

list
: every element, its unit and whether it can be read on this machine

A verb with no element reports every element of its group, one line each.
Run rptop help VERB to read about a verb.

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
