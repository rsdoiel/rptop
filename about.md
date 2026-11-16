---
title: rptop
abstract: |-
  A vitals dashboard for a Raspberry Pi 5 with an optional AI HAT+ 2: a terminal dashboard and a verb-oriented command line over the same collectors, so each readout (CPU, memory, temperatures, throttling, fan, disk, network) is also a scriptable report of one value and its unit, with JSONL recording for comparing runs.
authors:
  - family_name: Doiel
    given_name: R. S.
    id: https://orcid.org/0000-0003-0900-6903



repository_code: https://github.com/rsdoiel/rptop
version: 0.0.1
license_url: https://www.gnu.org/licenses/agpl-3.0.txt
operating_system:
  - Raspberry Pi OS (64-bit, Debian 13 trixie)

programming_language:
  - Go >= 1.27.3

keywords:
  - Raspberry Pi
  - monitoring
  - dashboard
  - TUI
  - CLI
  - Hailo

date_released: 2026-11-15
---

About this software
===================

## rptop 0.0.1

First release. A vitals dashboard and scriptable reporter for a Raspberry Pi 5 with an optional AI HAT+ 2: a terminal dashboard (bare rptop; keys q r R m p ?; options -interval, -warn-temp, -crit-temp, -out; no configuration file), reading verbs temp, cpu, mem, fan, volts, disk, net, throttle and list with -n, -j, -above and -below and the workspace exit codes, and JSONL recording (rptop record, or r in the dashboard) with a meta line, samples, markers and a meta_end line, a free-space guard and flat element names (DR-0003). The Hailo temperature is read in process through libhailort behind the hailo build tag (HailoRT >= 5.4.0), so it comes from a source build: the release archive is built without the tag and cgo, runs on any Pi 5 without HailoRT, and reports the Hailo element absent. Linux arm64 on a Raspberry Pi 5 only.

## Authors

- [R. S. Doiel](https://orcid.org/0000-0003-0900-6903)






A vitals dashboard for a Raspberry Pi 5 with an optional AI HAT+ 2: a terminal dashboard and a verb-oriented command line over the same collectors, so each readout (CPU, memory, temperatures, throttling, fan, disk, network) is also a scriptable report of one value and its unit, with JSONL recording for comparing runs.

- [License](https://www.gnu.org/licenses/agpl-3.0.txt)
- [Code Repository](https://github.com/rsdoiel/rptop)
  - [Issue Tracker](https://github.com/rsdoiel/rptop/issues)

## Programming languages

- Go >= 1.27.3


## Operating Systems

- Raspberry Pi OS (64-bit, Debian 13 trixie)


## Software Requirements

- Go >= 1.27.3
- charm.land/bubbletea/v2 >= 2.1.0
- charm.land/bubbles/v2 >= 2.2.1
- charm.land/lipgloss/v2 >= 2.0.6
- github.com/charmbracelet/x/ansi >= 0.11.9
- github.com/charmbracelet/x/term >= 0.2.2


## Software Suggestions

- CMTools >= 0.0.46
- Pandoc >= 3.1.11
- GNU Make >= 3
- vcgencmd (libraspberrypi-bin), for the PMIC temperature, the core voltage and throttling
- HailoRT >= 5.4.0
- A C compiler (GCC or Clang) for the Hailo temperature
- jq, zip and the GitHub CLI (gh), to run make check-release, make release and release.bash


