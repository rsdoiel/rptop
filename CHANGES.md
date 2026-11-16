# Changes

## v0.0.1 — 2026-11-15

The first release. A vitals dashboard and scriptable reporter for a Raspberry Pi 5 with an optional AI HAT+ 2.

### Added

- **The dashboard,** a bare `rptop` in a terminal: CPU and per-core load, memory, temperatures with sparklines coloured by
  `-warn-temp` and `-crit-temp`, throttling, fan, core voltage, disk and network throughput. Keys `q`, `r`, `R`, `m`, `p`,
  `?`. Options `-i`/`-interval`, `-warn-temp`, `-crit-temp` and `-out`; there is no configuration file. Without a terminal
  it prints help to standard error and exits 2.
- **Reading verbs:** `temp`, `cpu`, `mem`, `fan`, `volts`, `disk`, `net`, `throttle` and `list`. Each element reports one
  value and its unit (`-n` for the bare number, `-j` for JSON); `-above` and `-below` print the reading and answer with the
  exit code; a verb with no element reports its group under the bulk rule.
- **Recording:** `rptop record` and the dashboard's `r` key write one JSONL file per run, with a `meta` line, a `sample`
  line per interval, `marker` lines and a `meta_end` line on every ending. Flat element names, `null` for an unreadable
  reading, a free-space guard of 64 MiB. The format is decision record DR-0003.
- **The Hailo temperature,** read in process through libhailort behind the `hailo` build tag (HailoRT >= 5.4.0). On HailoRT
  5.4.0 the device is opened with `hailo_create_device_by_id`; `hailo_create_pcie_device` is Hailo-8 only.
- **The release archive is built without the `hailo` tag and without cgo,** so it runs on any Pi 5 with no HailoRT. In it
  `rptop temp hailo` reports the element absent; build from source for the Hailo temperature.
- **Exit codes** from the workspace table, one class per failure: 66 for an element not on this machine, 69 for one that
  cannot be reached, 65 for text that cannot be parsed, 73 and 74 for recordings.
- **Man pages** for `rptop` and each verb, generated from the help text.

### Known limits

- Linux arm64 on a Raspberry Pi 5 only.
- The race detector has not been run (it does not run on the Pi's kernel).
- The DuckDB recipe in the README has not been run on the Pi.
