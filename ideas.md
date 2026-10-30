# rptop: Project Proposal (v0.1)

## Summary

| Item | Detail |
|---|---|
| **Name** | rptop |
| **Purpose** | A Raspberry Pi 5 vitals dashboard in the terminal, with start/stop JSONL recording for reviewing machine activity and comparing the impact of different software |
| **Target hardware** | Raspberry Pi 5 + AI HAT+ 2 (Hailo-10H), Raspberry Pi OS (64-bit) |
| **Language / stack** | Go, Bubble Tea, Lip Gloss, Bubbles, gopsutil |
| **In scope (v0.1)** | Live dashboard, temperatures in °C, throttling status, JSONL recording (dashboard toggle and CLI verbs), headless mode, control socket |
| **Out of scope (v0.1)** | Process table, tree view, kill/renice, mouse support |
| **Estimated effort** | **About 3 weekends** for a solid v0.1 (2 to 3 if the control socket is deferred) |

---

## Goals and Non-Goals

### Goals

- Show Pi vitals in near realtime (default **1 s** refresh, configurable).
- Report **CPU/SoC, PMIC, and Hailo-10H temperatures in Celsius**.
- Record samples to a **JSONL file** that can be started and stopped from the dashboard or from the command line.
- Run **headless** (no TUI) for long recordings under systemd or tmux.
- Never block the UI on slow sensor calls.

### Non-goals

- Replacing htop's process management.
- Supporting non-Pi hardware (it should degrade gracefully, but isn't the target).

---

## Dashboard Layout

A single screen, with panels arranged by Lip Gloss:

```
┌ rptop ─ raspberrypi ─ up 3d 4h ─ rec ● rptop-20261029-143012.jsonl ┐
│ CPU  [██████░░░░░░░░░░] 38%   freq 2400 MHz   load 0.9 0.7 0.5     │
│  c0 [████░░░░] 31%   c1 [██████░░] 45%   c2 [███░░░░░] 22% ...     │
│ MEM  [███████░░░░░░░░░] 2.1G / 8.0G   SWAP 0 / 2.0G                │
├ Temperatures (°C) ─────────────────────────────────────────────────┤
│ SoC   54.0  ▁▂▃▃▄▅▅▆▆▅   PMIC 49.2   Hailo 61.5  ▂▃▄▅▅▆▆▆▇▇        │
├ Power / Health ────────────────────────────────────────────────────┤
│ Throttled: none   Fan: 3200 RPM   Core V: 0.88 V                   │
├ Disk / Net ────────────────────────────────────────────────────────┤
│ nvme0 R 12 MB/s W 3 MB/s     eth0 ↓ 1.2 MB/s ↑ 0.1 MB/s            │
└ q quit  r rec on/off  R rec+label  m marker  p pause  ? help ──────┘
```

### Panels

1. **CPU:** total and per-core utilization, current frequency, load average.
2. **Memory:** used/total, swap, optionally cached/buffers.
3. **Temperatures:** SoC, PMIC, Hailo-10H, each with a **sparkline of recent history** and color thresholds (green below 60, yellow 60 to 80, red above 80, configurable).
4. **Power and health:** throttling flags, fan RPM, core voltage.
5. **I/O:** disk and network throughput.
6. **Accelerator (stretch):** Hailo utilization or inference stats if HailoRT exposes them.
7. **Header:** hostname, uptime, and recording state (`rec ●` with filename while active, `rec ○ off` when stopped).

---

## Data Sources

| Metric | Source | Notes |
|---|---|---|
| CPU util, load, mem, swap, disk, net | **gopsutil** (or `/proc` directly) | Cross-checked against `/proc/stat` |
| CPU frequency | `/sys/devices/system/cpu/cpu0/cpufreq/scaling_cur_freq` | Value in kHz |
| SoC temperature | `/sys/class/thermal/thermal_zone0/temp` | Millidegrees, divide by 1000 |
| PMIC temperature | `vcgencmd measure_temp pmic` | Shell-out, output like `temp=49.2'C` |
| Throttling | `vcgencmd get_throttled` | Hex bitmask; decode into named flags |
| Fan RPM | `/sys/class/hwmon/*/fan1_input` | Present when the official active cooler is attached |
| Core voltage | `vcgencmd measure_volts core` | Optional |
| Hailo-10H temperature | `hailortcli` or HailoRT | **To be verified on your device** |

On the Pi, `vcgencmd measure_temp` and the sysfs thermal zone report the same SoC temperature, so there is no separate GPU sensor. The Hailo reading is the one open question, and Phase 0 exists to settle it.

---

## Architecture

### Package layout

```
rptop/
├── cmd/rptop/main.go        # verbs, flags, wiring
├── internal/
│   ├── collector/           # one file per source, shared interface
│   │   ├── cpu.go
│   │   ├── mem.go
│   │   ├── thermal.go
│   │   ├── vcgencmd.go
│   │   ├── hailo.go
│   │   └── io.go
│   ├── sample/              # Sample struct + JSON encoding
│   ├── recorder/            # RecorderController + JSONL writer
│   ├── control/             # Unix socket server/client
│   ├── ui/                  # Bubble Tea model, views, styles
│   └── config/              # flags + optional TOML/YAML
└── README.md
```

### Key design decisions

- **Collectors are independent and run on their own goroutines**, each with its own timeout. A slow `hailortcli` call can never freeze the UI or delay the CPU reading. Each collector publishes its latest value, and a single sampler assembles them into a `Sample` on every tick.
- **One sampler feeds two consumers:** the TUI (always) and the recorder (only while recording). What you see is exactly what gets recorded.
- **A missing sensor isn't an error.** If a collector can't read a value, the field is `null` in JSON and shown as `n/a` in the UI.
- **A `Collector` interface** (`Name()`, `Collect(ctx) (map[string]any, error)`) keeps adding new sensors cheap.
- **Bubble Tea integration:** a `tea.Tick` drives refreshes, and the sampler sends results as messages. The model stores a ring buffer per metric for sparklines.
- **Slow sensors are polled less often** (PMIC, voltage) and cached, to limit shell-out overhead.

### RecorderController

The recorder is a start/stoppable subscriber owned by a `RecorderController` that exposes `Start(label, path)`, `Stop()`, and `Status()`. It has two states, `idle` and `recording`.

- When idle, the cost is effectively zero, because samples aren't forwarded.
- The **TUI key handler, the headless CLI verb, and the control socket** all call the same controller, so behavior is identical however recording is toggled.
- The TUI sends `StartRecording` and `StopRecording` messages, and the controller reports state changes back as messages so the header updates immediately.
- If a write fails (disk full, for example), the controller stops itself and the dashboard shows an error banner instead of silently losing data.

---

## Recording

### Control paths

| Control path | How it works |
|---|---|
| **Dashboard toggle** | **`r`** starts or stops recording. **`R`** opens a small Bubbles `textinput` to enter a label first. |
| **`rptop record`** | Runs headless (no TUI) and writes JSONL until Ctrl-C, SIGTERM, or `--duration` elapses. |
| **`rptop --record`** | Opens the dashboard with recording already on. Pressing `r` stops it. |
| **`rptop start` / `rptop stop`** | Controls recording in an already-running instance (dashboard or daemon) over a Unix socket. |
| **`rptop status`** | Prints whether recording is active, the current file, sample count, and elapsed time. |
| **`rptop daemon`** (optional) | Headless, no recording until told to. Useful under systemd so `rptop start --label "after-install"` works without restarting anything. |

### Behavior rules

- **Stopping the recording never closes the dashboard.** The dashboard keeps sampling for display, and the recorder is simply detached from the sample stream.
- **Each start creates a new file**, for example `rptop-20261029-143012.jsonl`, so every run is a clean, labeled unit. Pressing `r` again starts a fresh file rather than appending.
- **On stop, the recorder flushes and closes the file**, then writes a final `"type":"meta_end"` line with sample count, duration, and a clean-shutdown flag. A file lacking this line tells you the process crashed or lost power.
- A **header line** (`"type":"meta"`) at the start of each file records hostname, kernel version, rptop version, interval, label, and sensor availability, so a file can be interpreted months later.
- Records are flushed every sample (or every N samples), so a crash or power loss loses little.
- Optional **rotation** by size or duration.

### Record schema

Every line is self-contained, using ISO 8601 UTC timestamps and explicit units in field names:

```json
{"ts":"2026-10-29T14:30:12.004Z","seq":42,"run":"before-ollama-install","interval_ms":1000,
 "cpu":{"total_pct":38.2,"per_core_pct":[31.0,45.1,22.4,54.3],"freq_mhz":2400,"load1":0.9,"load5":0.7,"load15":0.5},
 "mem":{"used_bytes":2254857830,"total_bytes":8589934592,"swap_used_bytes":0},
 "temp_c":{"soc":54.0,"pmic":49.2,"hailo":61.5},
 "power":{"throttled_hex":"0x0","throttled_flags":[],"core_volts":0.88},
 "fan_rpm":3200,
 "disk":{"read_bps":12582912,"write_bps":3145728},
 "net":{"rx_bps":1258291,"tx_bps":104857},
 "self":{"cpu_pct":0.6,"rss_bytes":18874368}}
```

The `self` block records rptop's own CPU and memory use so you can subtract its overhead when comparing runs.

### Why JSONL fits

It's append-only and crash-tolerant, trivially streamable with `tail -f`, and easy to analyze with **`jq`**, **DuckDB** (`SELECT ... FROM read_json_auto('*.jsonl')`), pandas, or Grafana via a small converter. For comparing software impact, record two labeled runs and diff averages and peaks.

```bash
# Peak Hailo temperature in a run
jq -s 'map(select(.temp_c.hailo)) | max_by(.temp_c.hailo) | .temp_c.hailo' run.jsonl

# Average CPU utilization
jq -s 'map(.cpu.total_pct) | add/length' run.jsonl
```

### Control socket

For `start`, `stop`, and `status` to work against a live instance, rptop listens on a **Unix domain socket**, for example `$XDG_RUNTIME_DIR/rptop.sock` (or `/run/rptop.sock` when running as a service).

- The protocol is a few newline-delimited JSON commands: `{"cmd":"start","label":"after-install"}` returns `{"ok":true,"file":"..."}`.
- Permissions are restricted to the owning user, so there is no network exposure.
- If no instance is running, `rptop start` prints a clear error. Auto-launching a daemon can come later.

### Edge cases

| Situation | Behavior |
|---|---|
| `r` pressed while a CLI-started recording is active | Stops it. State is shared, so the dashboard reflects reality. |
| `rptop start` when already recording | Returns an error naming the current file. A new run requires `stop` first (or a `--restart` flag). |
| `--duration` expires while the dashboard is open | Recording stops and closes cleanly. The dashboard stays up and shows "recording finished". |
| SIGINT or SIGTERM during recording | Flush, write the `meta_end` line, close the file, remove the socket, exit. |
| Disk nearly full | Warn at a threshold (say under 100 MB free) and stop recording before filling the disk. |
| Marker pressed while idle | Ignored with a brief status message. |

---

## Configuration and CLI

| Flag | Default | Purpose |
|---|---|---|
| `--interval` | `1s` | Sampling and refresh interval |
| `--record` | off | Start recording immediately when the dashboard opens |
| `--out` | `./rptop-<timestamp>.jsonl` | Output path or directory |
| `--label` | none | Run label stored in meta and every record |
| `--duration` | unlimited | Auto-stop recording after a period |
| `--warn-temp`, `--crit-temp` | 60 / 80 | Color thresholds in °C |
| `--hailo-cmd` | auto | Override how Hailo temperature is read |

---

## Milestones

| Phase | Deliverable | Effort |
|---|---|---|
| **0. Discovery** | Confirm how to read Hailo-10H temperature, fan, and voltage on your Pi; capture sample outputs as test fixtures | 1 to 2 hours |
| **1. Collectors + sample struct** | All sensors readable, unit-tested against fixtures, timeouts working | Half to 1 weekend |
| **2. TUI dashboard** | Bubble Tea model, panels, sparklines, color thresholds, key bindings | 1 weekend |
| **3. Recorder** | `RecorderController`, JSONL writer, meta and meta_end lines, headless `rptop record`, `r`/`R` keys, labels, rotation | 1 weekend |
| **4. Control and polish** | Unix socket with `start`/`stop`/`status`, optional `daemon`, config file, README with `jq` and DuckDB recipes, systemd unit example, `go install` instructions | Half to 1 weekend |

If the schedule needs trimming, the socket and the `start`/`stop`/`status`/`daemon` verbs are the easiest items to defer to v0.2. The dashboard toggle and `rptop record` alone already satisfy the core requirement, and the controller design doesn't change.

---

## Risks and Mitigations

| Risk | Mitigation |
|---|---|
| Hailo temperature access method is unclear or requires root or a specific driver | Do Phase 0 first; fall back to a configurable external command (`--hailo-cmd`) |
| `vcgencmd` shell-outs add CPU overhead at fast intervals | Poll slow sensors less often than 1 Hz and cache results |
| The monitor skews the measurements it takes | Keep rptop lightweight and record its own usage in the `self` block |
| JSONL files grow large | About 500 bytes per line at 1 Hz is roughly 40 MB per day; rotation and optional gzip handle this |
| Sensor paths differ between OS versions | Detect at startup and report availability in the meta header |

---

## Stretch Ideas (post v0.1)

- **`rptop compare a.jsonl b.jsonl`** prints average, p95, and peak deltas per metric, directly serving the software-impact use case.
- **Event markers:** `m` in the TUI (or a `rptop mark "text"` verb) inserts a `"type":"marker"` line, such as "started model benchmark", so timelines line up later.
- Hailo utilization and inference throughput, if exposed.
- Prometheus exporter mode.
- Top-N process summary panel, a small step toward htop without the full process table.

---

## Open Questions

1. **Which OS image are you on** (Raspberry Pi OS Bookworm or the newer Trixie)? It affects sensor paths and `vcgencmd` availability.
2. **Do you have the official Active Cooler** or another fan? That determines whether fan RPM is worth including.
3. **What sampling rate do you want while recording**, 1 s or faster? Sub-second works for fast sensors, but the shell-outs would need to be sampled more slowly.
4. **Will you run rptop mostly interactively or as a background service?** If it's a service, I'd move the daemon and socket firmly into v0.1.
5. **Should markers go in v0.1?** They're cheap and make before-and-after comparisons much more useful.

I can start on the Phase 0 discovery script and the `Sample` struct next, since those two pieces determine the rest of the design.
