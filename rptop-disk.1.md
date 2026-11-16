%rptop-disk(1) user manual | version 0.0.1 bb5682a
% R. S. Doiel
% 2026-11-15

# NAME

rptop-disk — disk throughput in bytes per second, summed over the whole physical disks.

# SYNOPSIS

rptop disk [read|write] [-interval D] [-n] [-j] [-above X] [-below X]

# DESCRIPTION

Disk throughput in bytes per second, summed over the whole physical disks.

## Elements

read
: bytes read per second

write
: bytes written per second

# OPTIONS

-h, -help
: display help

-n, -number
: print the bare number, without its unit

-j, -json
: print JSON, for example {"name":"temp.soc","value":54.6,"unit":"°C"}; a failure is also an error object with its class and code

-above X
: exit 1 unless the reading is above X; the reading is still printed

-below X
: exit 1 unless the reading is below X; the reading is still printed

-i, -interval D
: time between the two samples, a Go duration; default 1s

Partitions, ram, loop and zram devices are not counted. Throughput is the difference between two samples of /proc/diskstats.

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
rptop disk write -i 5s
~~~
