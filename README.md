# rptop

A vitals dashboard for a Raspberry Pi 5 with an optional AI HAT+ 2.

rptop has two front ends over the same collectors: a terminal dashboard, and a
verb-oriented command line where each element (a temperature, the fan speed,
the throttling state) is a verb that reports one value and its unit. That makes
every readout usable from shell scripts, cron and systemd.

Status: concept. This repository is a scaffold; the `rptop` command parses its
standard options (`-h`, `-l`, `-v`) and has no verbs yet.

## Requirements

- Go >= 1.27.3
- A Raspberry Pi 5 running Raspberry Pi OS (64-bit), Debian 13 (trixie). rptop is specific to the Pi and is not
  cross-platform; a release is source, or source and a Linux arm64 binary.

## Build

~~~shell
make build
make test
~~~

## Exit status

rptop uses the workspace exit codes (0 ok, 1 negative answer, 2 usage, 65 data,
66 no input, 69 unavailable, 70 internal, 73 cant create, 74 I/O, 75 temp fail,
77 no permission, 78 config). See `rptop -help`.

## License

AGPL-3.0
