Installation **rptop**
======================

**rptop** is a vitals dashboard and scriptable reporter for a Raspberry Pi 5, with an optional AI HAT+ 2. It runs on a
Raspberry Pi 5 with Raspberry Pi OS (64-bit), Debian 13 (trixie), and nowhere else: a release is source, or source and a
Linux arm64 binary. There are no macOS or Windows builds.

Quick install with curl
-----------------------

On the Pi, the installer downloads the Linux arm64 archive of the latest release and puts `rptop` in `$HOME/bin`:

~~~shell
curl https://rsdoiel.github.io/rptop/installer.sh | sh
~~~

Make sure `$HOME/bin` is in your `PATH`. The archive is `rptop-vX.Y.Z-Linux-aarch64.zip`; the installer is generated and
looks for exactly that name, so it fails with a 404 on any other system.

Installing from source
----------------------

### Required software

- Go >= 1.27.3
- GNU Make >= 3
- Pandoc >= 3.9, to build the man pages (optional; `make install` skips pages that were not built)
- For the Hailo temperature (optional): HailoRT >= 5.4.0 with its headers (`/usr/include/hailo/hailort.h`) and a C compiler

The Makefile builds with the `hailo` tag when it finds `/usr/include/hailo/hailort.h`, and without it otherwise. Without
the tag nothing needs libhailort and `rptop temp hailo` reports the element absent (exit 66). Force a plain build with
`make GO_TAGS=`.

### Steps

1. git clone https://github.com/rsdoiel/rptop
2. Change directory into the `rptop` directory
3. Make to build, test and install

~~~shell
git clone https://github.com/rsdoiel/rptop
cd rptop
make
make test
make install
~~~

`make install` puts `rptop` in `$HOME/bin` and the man pages in `$HOME/man/man1` (`rptop.1` and one `rptop-VERB.1` for
each verb). Use `make install prefix=/usr/local` for another place, and make sure `bin` is on your `PATH` and `man` on
your `MANPATH`. `make uninstall` removes them.

### Check it works

~~~shell
rptop -version
rptop list          # every element, and whether this machine can read it
rptop temp soc
rptop               # the dashboard, in a terminal at least 72 columns by 20 rows
~~~

`rptop list` shows `no_input` for an element this machine does not have (no fan, no Hailo device, built without the tag)
and `unavailable` for one that is present but cannot be reached.

Notes
-----

- **No daemon, no configuration file, no network.** rptop reads `/sys`, `/proc`, `vcgencmd` and, with the tag, the Hailo
  device through libhailort, and writes only the recordings you ask for.
- **Reading the Hailo temperature opens the device.** It shares the device with hailo-ollama; a handle held open by rptop
  was tested beside a running inference on HailoRT 5.4.0 without a problem.
- **`vcgencmd`** is in the `libraspberrypi-bin` package on Raspberry Pi OS. Without it the PMIC temperature, the core
  voltage and `throttle` report the element absent.
