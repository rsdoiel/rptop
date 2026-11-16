package cli

import (
	"encoding/json"
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

// requirementsCheck runs `make check-requirements` in a copy of the files it reads,
// after change has edited the copy of codemeta.json.
func requirementsCheck(t *testing.T, change func(reqs []string) []string) (string, error) {
	t.Helper()
	dir := t.TempDir()
	for _, f := range []string{"Makefile", "go.mod", "go.sum"} {
		b, err := os.ReadFile(filepath.Join("../..", f))
		if err != nil {
			t.Fatal(err)
		}
		os.WriteFile(filepath.Join(dir, f), b, 0o644)
	}
	raw, err := os.ReadFile("../../codemeta.json")
	if err != nil {
		t.Fatal(err)
	}
	var doc map[string]any
	if err := json.Unmarshal(raw, &doc); err != nil {
		t.Fatal(err)
	}
	var reqs []string
	for _, r := range doc["softwareRequirements"].([]any) {
		reqs = append(reqs, r.(string))
	}
	doc["softwareRequirements"] = change(reqs)
	out, _ := json.Marshal(doc)
	os.WriteFile(filepath.Join(dir, "codemeta.json"), out, 0o644)
	b, err := exec.Command("make", "-s", "-C", dir, "check-requirements").CombinedOutput()
	return string(b), err
}

func without(reqs []string, prefix string) []string {
	var out []string
	for _, r := range reqs {
		if !strings.HasPrefix(r, prefix) {
			out = append(out, r)
		}
	}
	return out
}

// softwareRequirements and go.mod must agree in both directions (a dependency that
// was missing from the list was not noticed before).
func TestCheckRequirements(t *testing.T) {
	if out, err := requirementsCheck(t, func(r []string) []string { return r }); err != nil || !strings.Contains(out, "consistent") {
		t.Errorf("the real files must pass: %v\n%s", err, out)
	}
	cases := []struct {
		name   string
		change func([]string) []string
		want   []string
	}{
		{"a direct dependency is not listed",
			func(r []string) []string { return without(r, "charm.land/lipgloss/v2") },
			[]string{"charm.land/lipgloss/v2", "does not list"}},
		{"a listed version differs from go.mod",
			func(r []string) []string {
				return append(without(r, "charm.land/bubbletea/v2"), "charm.land/bubbletea/v2 >= 2.0.0")
			},
			[]string{"charm.land/bubbletea/v2", "go.mod requires v2.1.0"}},
		{"a listed module is not in go.mod",
			func(r []string) []string { return append(r, "example.com/ghost >= 1.0.0") },
			[]string{"example.com/ghost", "go.mod does not require"}},
	}
	for _, c := range cases {
		out, err := requirementsCheck(t, c.change)
		if err == nil {
			t.Errorf("%s: check-requirements passed\n%s", c.name, out)
			continue
		}
		for _, w := range c.want {
			if !strings.Contains(out, w) {
				t.Errorf("%s: output lacks %q:\n%s", c.name, w, out)
			}
		}
	}
}

// The release archive is built without the hailo tag and without cgo: with the
// tag the binary needs libhailort.so.5.4.0 and will not start on a Pi without it.
func TestReleaseArchiveHasNoHailoDependency(t *testing.T) {
	b, err := os.ReadFile("../../Makefile")
	if err != nil {
		t.Fatal(err)
	}
	m := regexp.MustCompile(`(?m)^dist/Linux-aarch64:.*\n((?:\t.*\n)+)`).FindSubmatch(b)
	if m == nil {
		t.Fatal("no dist/Linux-aarch64 target")
	}
	recipe := string(m[1])
	if strings.Contains(recipe, "GO_TAGS") || strings.Contains(recipe, "-tags") {
		t.Errorf("the release archive is built with build tags:\n%s", recipe)
	}
	if !strings.Contains(recipe, "CGO_ENABLED=0") {
		t.Errorf("the release archive is not built with CGO_ENABLED=0:\n%s", recipe)
	}
}
