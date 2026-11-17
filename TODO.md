
# Action Items

## Modernization (2026-11-16)

To match the workspace standard (knowledge and opml are the models; Go 1.27.3 across repositories). Do one repository at a time; run the tests and `make -n release` after each step.

- [x] INSTALL_NOTES_macOS.md and INSTALL_NOTES_Windows.md: not applicable. rptop is Raspberry Pi only (Linux aarch64), so the macOS and Windows notes would only mislead; the one line in the generated installer.sh that names the macOS notes is the cmt template and prints only on macOS.
