package cli

import (
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"
)

// The Makefile's VERBS list drives `make verb-help`, which writes one man page
// source per verb; it must name exactly the verbs that have a help page.
func TestMakefileVerbsMatchHelpPages(t *testing.T) {
	b, err := os.ReadFile("../../Makefile")
	if err != nil {
		t.Fatal(err)
	}
	m := regexp.MustCompile(`(?m)^VERBS\s*=(.*)$`).FindSubmatch(b)
	if m == nil {
		t.Fatal("no VERBS line in the Makefile")
	}
	got := strings.Fields(string(m[1]))
	var want []string
	for v := range verbHelp {
		want = append(want, v)
	}
	sort.Strings(got)
	sort.Strings(want)
	if strings.Join(got, " ") != strings.Join(want, " ") {
		t.Errorf("Makefile VERBS = %v, help pages = %v", got, want)
	}
}

// The man page list is named, not globbed: a .1.md that is missing must stay in
// MAN_PAGES_1 so that `make man` fails, instead of the page quietly dropping
// out of `make man` and `make install` while the build succeeds.
func TestMakefileManPagesDoNotDependOnTheFilesPresent(t *testing.T) {
	dir := t.TempDir()
	mk, err := os.ReadFile("../../Makefile")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "Makefile"), mk, 0o644); err != nil {
		t.Fatal(err)
	}
	for v := range verbHelp {
		if v == "temp" {
			continue // the page whose source is missing
		}
		os.WriteFile(filepath.Join(dir, "rptop-"+v+".1.md"), []byte("x\n"), 0o644)
	}
	os.WriteFile(filepath.Join(dir, "rptop.1.md"), []byte("x\n"), 0o644)

	// A recipe is expanded when it runs, so this prints the value make really uses.
	out, err := exec.Command("make", "-s", "-C", dir, "--eval=print-man-pages: ; @echo $(MAN_PAGES_1)", "print-man-pages").Output()
	if err != nil {
		t.Fatalf("make: %v", err)
	}
	got := strings.Fields(string(out))
	want := []string{"rptop.1"}
	for v := range verbHelp {
		want = append(want, "rptop-"+v+".1")
	}
	sort.Strings(got)
	sort.Strings(want)
	if strings.Join(got, " ") != strings.Join(want, " ") {
		t.Errorf("MAN_PAGES_1 = %v\nwant           %v\n(rptop-temp.1.md is absent and must still be listed)", got, want)
	}
}

// `make release` must not commit or push: release.bash is the only place that does.
func TestMakefileReleaseDoesNotCommitOrPush(t *testing.T) {
	b, err := os.ReadFile("../../Makefile")
	if err != nil {
		t.Fatal(err)
	}
	m := regexp.MustCompile(`(?m)^release:(.*)$`).FindSubmatch(b)
	if m == nil {
		t.Fatal("no release target")
	}
	for _, dep := range strings.Fields(string(m[1])) {
		if dep == "save" || dep == "publish" || dep == "refresh" {
			t.Errorf("the release target depends on %q, which commits or pushes", dep)
		}
	}
}
