# rptop

A vitals dashboard and scriptable reporter for a Raspberry Pi 5, with an optional AI HAT+ 2 (Hailo-10H).

rptop has three faces over one set of collectors:

- **a terminal dashboard** (a bare `rptop`): CPU and per-core load, memory, temperatures with sparklines,
  throttling, fan, core voltage, disk and network throughput;
- **a verb-oriented command line** (`rptop temp soc`, `rptop fan`, `rptop throttle`): each element reports one value
  and its unit, so every readout works in shell scripts, cron and systemd;
- **a recorder** (`rptop record`, or `r` in the dashboard): one JSONL file per run, to compare the machine before and
  after a change.

It is specific to the Raspberry Pi 5 on Debian 13 (trixie) and is not cross-platform. There is no daemon and no
configuration file.

Status: pre-release. Everything described here is built and tested; see [CHANGES.md](CHANGES.md).

~~~text
┌ rptop ─ harvey ─ rec ● rptop-20261115-143012.jsonl 12 samples 0:20 ──────────┐
│ CPU  [██████░░░░░░░░░░]  37.0%  2400 MHz  load 0.90                          │
│   c0 [█░░░]  31.0% c1 [██░░]  45.1% c2 [█░░░]  22.4% c3 [██░░]  54.3%        │
│      ▁▂▂▃▃▄▄▅▅▆▆▇▇█▁▂▂▃▄▄                                                    │
│ MEM  [██░░░░░░░░░░░░░░] 2.1G / 15.8G  swap 0 B                               │
├ Temperatures ────────────────────────────────────────────────────────────────┤
│ SoC     59.0 °C  ▁▂▃▃▄▅▆▆▇█▁▂▃▃▄▅▆▆▇█                                        │
│ PMIC    49.2 °C  ▁▁▁▁▁▁▁▁▁▁▁▁▁▁▁▁▁▁▁▁                                        │
│ Hailo   68.3 °C  ▁▁▂▂▂▃▃▄▄▄▅▅▅▆▆▇▇▇██                                        │
│ RP1     51.4 °C  ▁▁▁▁▁▁▁▁▁▁▁▁▁▁▁▁▁▁▁▁                                        │
├ Power / Health ──────────────────────────────────────────────────────────────┤
│ Throttle  now: none                                                          │
│ since boot: arm frequency capped, throttled, soft temperature limit          │
│ Fan  3725 rpm     Core  0.7838 V                                             │
├ Disk / Net ──────────────────────────────────────────────────────────────────┤
│ Disk  read 12.0 MB/s   write 3.0 MB/s                                        │
│ Net   rx 1.2 MB/s   tx 102.4 KB/s                                            │
└ q quit  r record  R label  m marker  p pause  ? help ────────────────────────┘
~~~

## Requirements

- Go >= 1.27.3
- A Raspberry Pi 5 running Raspberry Pi OS (64-bit), Debian 13 (trixie)
- Optional: an AI HAT+ 2 with HailoRT >= 5.4.0 and a C compiler, for the Hailo temperature. This needs a **build from
  source** with the `hailo` tag, which the Makefile enables when `/usr/include/hailo/hailort.h` exists. **The release
  archive is built without it** so that it runs on any Pi 5: there, `rptop temp hailo` reports the element absent (exit 66).
- Optional: `vcgencmd` (the PMIC temperature, core voltage and throttling); a fan on the official active cooler for `fan`.

See [INSTALL.md](INSTALL.md) for building and installing.

## The dashboard

Run `rptop` in a terminal at least 72 columns by 20 rows. Without a terminal (a pipe, a redirect, cron) it prints the
help page to standard error and exits 2.

| Option | Default | Meaning |
|---|---|---|
| `-i`, `-interval D` | `1s` | time between samples (a Go duration, at least `500ms`) |
| `-warn-temp C` | `60` | temperatures at or above this are yellow |
| `-crit-temp C` | `80` | temperatures at or above this are red (and above `-warn-temp`) |
| `-out PATH` | `.` | where recordings started with `r` go |

| Key | Action |
|---|---|
| `q`, `ctrl+c` | quit; an open recording is ended first |
| `r` | start a recording, or stop the open one |
| `R` | ask for a label, then start a recording |
| `m` | ask for marker text and write it into the recording |
| `p` | pause the display (sampling and recording go on) |
| `?` | show the keys |

A reading that cannot be taken shows `n/a`, never `0`. If a recording cannot start or has to stop (the file exists, the
disk is nearly full) the dashboard shows a banner and keeps running.

## Reading one value

`rptop VERB [ELEMENT]` prints one reading and its unit; with no element it prints the whole group.

~~~shell
rptop temp soc            # 54.6 °C
rptop temp                # soc, pmic, hailo, rp1, one line each
rptop cpu core 2          # 22.4 %      (named cpu.core2 in output)
rptop fan -n              # 3725        (the bare number)
rptop volts core -j       # {"name":"volts.core","value":0.7838,"unit":"V"}
rptop throttle            # now: none / since boot: arm frequency capped, ...
rptop list                # every element, its unit, and whether it can be read here
~~~

| Verb | Elements | Unit |
|---|---|---|
| `temp` | `soc`, `pmic`, `hailo`, `rp1` | °C |
| `cpu` | `total`, `core N`, `freq`, `load` | %, MHz, load average |
| `mem` | `used`, `total`, `swap` | bytes |
| `fan` | `speed` | rpm |
| `volts` | `core` | V |
| `disk` | `read`, `write` | bytes per second |
| `net` | `rx`, `tx` | bytes per second |
| `throttle` | flags now and since boot | none |

`cpu`, `disk` and `net` compare two samples and take `-i`/`-interval` (default `1s`).

A threshold check prints the reading and answers with the exit code, so one call can both log and gate:

~~~shell
if rptop temp soc -above 70; then echo "SoC is above 70 °C"; fi
rptop fan -below 1000 && echo "fan is slow"
~~~

`-above X` and `-below X` exit 0 when every given condition holds and 1 when one does not. `throttle` exits 1 only when
something is active **now**; `-since-boot` lets the history decide instead.

Run `rptop help VERB` or `man rptop-VERB` for each verb.

## Recording

~~~shell
rptop record -label before-ollama -duration 10m      # prints the file's path when it is done
f=$(rptop record -duration 60s -out runs/)           # runs/ is made if it does not exist
rptop record -i 500ms                                # until Ctrl-C or SIGTERM
~~~

Each run is one file, `rptop-YYYYMMDD-HHMMSS.jsonl` (UTC) unless `-out` names a file. An existing file is never appended
to or overwritten (exit 73). Every line has a `type`:

- `meta`: the first line: host, kernel, rptop version, the interval and every element with its unit and whether it was
  readable when the run began;
- `sample`: one per interval, with the element names as keys (`"temp.soc":54.1`); an unreadable reading is `null`;
- `marker`: a note written from the dashboard's `m` key;
- `meta_end`: the last line, whatever ended the run (`interrupt`, `duration`, `stopped`, `low_space`, `write_error`) with
  `clean` true or false. **A file without it was cut short by a crash or power loss.**

Below 64 MiB of free space the run ends with `low_space` and the command exits 74. The format is
decision record DR-0003 in this workspace's `agents/projects/rptop/decisions/`.

### jq

~~~shell
# mean CPU use over a run
jq -s '[.[]|select(.type=="sample")|.["cpu.total"]]|add/length' run.jsonl

# peak SoC and Hailo temperature for each label across several runs
jq -s '[.[]|select(.type=="sample")] | group_by(.label)
       | map({label: .[0].label, samples: length,
              peak_soc: (map(.["temp.soc"])|max),
              peak_hailo: (map(.["temp.hailo"]|select(.!=null))|max)})' rptop-*.jsonl

# was every file finished cleanly?
for f in rptop-*.jsonl; do tail -1 "$f" | jq -e '.type=="meta_end" and .clean' >/dev/null || echo "$f incomplete"; done

# the markers
jq -c 'select(.type=="marker")' run.jsonl

# a CSV of time, SoC temperature and CPU use
jq -r 'select(.type=="sample")|[.ts,.["temp.soc"],.["cpu.total"]]|@csv' run.jsonl
~~~

### DuckDB

Flat names make each element a column. This recipe has not been run on the Pi, where DuckDB is not installed.

~~~sql
SELECT label, avg("cpu.total") AS cpu_mean, max("temp.soc") AS soc_peak, max("temp.hailo") AS hailo_peak
FROM read_json_auto('rptop-*.jsonl')
WHERE type = 'sample'
GROUP BY label;
~~~

`self.cpu` and `self.rss` in every sample are rptop's own cost, so you can subtract it when comparing runs.

## Exit status

rptop uses the workspace exit codes.

| Code | Class | When |
|---|---|---|
| 0 | ok | a reading was printed and every condition holds; a recording ended normally; help, version |
| 1 | negative | `-above` or `-below` does not hold; `throttle` reports something active now |
| 2 | usage | a bad option, element or value; a bare `rptop` without a terminal |
| 65 | data | a sensor's text could not be parsed |
| 66 | no input | the element is not present on this machine |
| 69 | unavailable | the element is present but cannot be reached (the Hailo device) |
| 70 | internal | a bug |
| 73 | cant create | the recording file exists, or its directory cannot be made |
| 74 | io | a write failed, or free space fell below the guard |
| 77 | no permission | the operating system refused access |

With no element, a verb prints every element it can read, reports the others on standard error, and exits with the class
of the first failure. In `-j` mode a failure is also an object on standard output,
`{"error": "...", "class": "no_input", "code": 66}`.

## Build and test

~~~shell
make build      # bin/rptop and the man pages
make test
~~~

## License

AGPL-3.0. See [LICENSE](LICENSE).
