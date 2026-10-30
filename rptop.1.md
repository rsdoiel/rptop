%rptop(1) user manual | version 0.0.0 95fce46
% R. S. Doiel
% 2026-10-30

# NAME

rptop — a Raspberry Pi vitals dashboard and scriptable reporter

# SYNOPSIS

rptop [-h|-help] [-l|-license] [-v|-version]

rptop VERB [PARAMETERS...]

rptop help [VERB]

# DESCRIPTION

rptop reports the vitals of a Raspberry Pi 5, with an optional AI HAT+ 2:
CPU, memory, temperatures, throttling, fan, disk and network. A dashboard and a
verb-oriented command line read the same collectors, so every element on the
dashboard is also a verb that reports one value and its unit.

This is a pre-release scaffold. No verbs exist yet.

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
