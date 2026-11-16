%rptop-throttle(1) user manual | version 0.0.1 bb5682a
% R. S. Doiel
% 2026-11-15

# NAME

rptop-throttle — what the firmware is throttling now and what has happened since boot.

# SYNOPSIS

rptop throttle [-since-boot] [-j]

# DESCRIPTION

What the firmware is throttling now and what has happened since boot.

## Elements

under-voltage
: supply voltage dropped

arm frequency capped
: the ARM clock was capped

throttled
: the CPU was throttled

soft temperature limit
: the soft temperature limit was reached

# OPTIONS

-h, -help
: display help

-since-boot
: let the history decide the exit code instead of the present

-j, -json
: print {"name","raw","now":[],"since_boot":[]}

Prints two lines, now: and since boot:, each listing the flags or none. Exits 0 when nothing is active now and 1 when something is, so a script is not tripped by an event hours ago. There is no -n, -above or -below.

# EXIT STATUS

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

# EXAMPLES

~~~shell
rptop throttle || echo throttled right now
~~~
