%rptop-record(1) user manual | version 0.0.0 95fce46
% R. S. Doiel
% 2026-10-30

# NAME

rptop-record — record every element to a JSONL file until interrupted, in the foreground.

# SYNOPSIS

rptop record [-label TEXT] [-out PATH] [-interval D] [-duration D]

# DESCRIPTION

Record every element to a JSONL file until interrupted, in the foreground.

# OPTIONS

-h, -help
: display help

-label TEXT
: a name for the run, stored on every line; use it to compare runs

-out PATH
: a file, or an existing directory; the default is rptop-YYYYMMDD-HHMMSS.jsonl (UTC) in the current directory. A directory that does not exist is made. An existing file is never appended to or overwritten

-i, -interval D
: time between samples, a Go duration; default 1s, at least 500ms

-duration D
: stop after this long; default until Ctrl-C or SIGTERM

Each run is one file: a meta line, one sample line per interval, and a meta_end line that is the last line whatever ended the run. A file with no meta_end was cut short by a crash or power loss. Keys are the element names of rptop list, a reading that cannot be taken is null, and the units are in the meta line. The first sample is written one interval after the start. If free space falls below 64 MiB the run ends with reason low_space and the command exits 74. The path of the finished file is printed on standard output; progress goes to standard error. See DR-0003 for the format.

# EXIT STATUS

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

# EXAMPLES

rptop record -label before-ollama -duration 10m

f=$(rptop record -duration 60s -out runs/) && jq -s 'map(.["temp.soc"]) | max' "$f"
