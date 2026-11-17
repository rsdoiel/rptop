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
	// The extra rule comes from a second makefile, not --eval, which GNU Make 3.81
	// (the one macOS ships) does not have.
	if err := os.WriteFile(filepath.Join(dir, "print.mk"), []byte("print-man-pages: ; @echo $(MAN_PAGES_1)\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	out, err := exec.Command("make", "-s", "-C", dir, "-f", "Makefile", "-f", "print.mk", "print-man-pages").Output()
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
		if dep == "generate" {
			t.Errorf("the release target depends on generate: it stamps new hashes and leaves the tree dirty; run make generate, commit, then make release")
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

// release builds from a commit that check-release has approved; it checks again
// afterwards that the build changed no tracked file.
func TestMakefileReleaseChecksBeforeAndAfter(t *testing.T) {
	b, err := os.ReadFile("../../Makefile")
	if err != nil {
		t.Fatal(err)
	}
	m := regexp.MustCompile(`(?m)^release:(.*)$`).FindSubmatch(b)
	if m == nil {
		t.Fatal("no release target")
	}
	deps := strings.Fields(string(m[1]))
	if len(deps) < 2 || deps[0] != "check-release" || deps[len(deps)-1] != "check-clean" {
		t.Errorf("release depends on %v; want check-release first and check-clean last", deps)
	}
	rel := regexp.MustCompile(`(?s)\ncheck-release:.*?\n\n`).Find(b)
	for _, want := range []string{"check-clean", "check-pushed", "check-generated"} {
		if !strings.Contains(string(rel), want) {
			t.Errorf("check-release does not run %s", want)
		}
	}
}

// git runs a git command in dir and fails the test if it fails.
func git(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), "GIT_AUTHOR_NAME=t", "GIT_AUTHOR_EMAIL=t@example.com", "GIT_COMMITTER_NAME=t", "GIT_COMMITTER_EMAIL=t@example.com")
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}
	return strings.TrimSpace(string(out))
}

// releaseRepo is a git repository with the Makefile, a codemeta.json and a bare
// origin, everything committed and pushed.
func releaseRepo(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	work, origin := filepath.Join(root, "work"), filepath.Join(root, "origin.git")
	os.MkdirAll(work, 0o755)
	git(t, root, "init", "-q", "--bare", "-b", "main", origin)
	git(t, work, "init", "-q", "-b", "main")
	git(t, work, "remote", "add", "origin", origin)
	mk, err := os.ReadFile("../../Makefile")
	if err != nil {
		t.Fatal(err)
	}
	os.WriteFile(filepath.Join(work, "Makefile"), mk, 0o644)
	os.WriteFile(filepath.Join(work, "codemeta.json"), []byte("{\n  \"version\": \"0.0.1\",\n  \"name\": \"rptop\"\n}\n"), 0o644)
	os.WriteFile(filepath.Join(work, ".gitignore"), []byte("*.tmp\ndist/\n"), 0o644)
	git(t, work, "add", "-A")
	git(t, work, "commit", "-q", "-m", "first")
	git(t, work, "push", "-q", "-u", "origin", "main")
	return work
}

func makeIn(dir, target string) (string, error) {
	out, err := exec.Command("make", "-s", "-C", dir, target).CombinedOutput()
	return string(out), err
}

// Everything must be committed so dist and the zips are built at a known commit.
func TestCheckClean(t *testing.T) {
	work := releaseRepo(t)
	if out, err := makeIn(work, "check-clean"); err != nil || !strings.Contains(out, "committed") {
		t.Fatalf("a clean tree must pass: %v\n%s", err, out)
	}
	os.WriteFile(filepath.Join(work, "scratch.tmp"), []byte("x"), 0o644) // ignored
	os.MkdirAll(filepath.Join(work, "dist"), 0o755)
	os.WriteFile(filepath.Join(work, "dist", "a.zip"), []byte("x"), 0o644) // ignored
	if out, err := makeIn(work, "check-clean"); err != nil {
		t.Errorf("ignored files must not count: %v\n%s", err, out)
	}
	os.WriteFile(filepath.Join(work, "codemeta.json"), []byte("{\"version\": \"0.0.2\"}\n"), 0o644)
	if out, err := makeIn(work, "check-clean"); err == nil || !strings.Contains(out, "codemeta.json") {
		t.Errorf("a modified tracked file must fail and be named: %v\n%s", err, out)
	}
	git(t, work, "checkout", "--", "codemeta.json")
	os.WriteFile(filepath.Join(work, "new.go"), []byte("package x\n"), 0o644)
	if out, err := makeIn(work, "check-clean"); err == nil || !strings.Contains(out, "new.go") {
		t.Errorf("an untracked file must fail and be named: %v\n%s", err, out)
	}
}

// The tag lands on the head of the branch on GitHub, so that must be the commit
// the zips were built from.
func TestCheckPushed(t *testing.T) {
	work := releaseRepo(t)
	if out, err := makeIn(work, "check-pushed"); err != nil {
		t.Fatalf("a pushed HEAD must pass: %v\n%s", err, out)
	}
	os.WriteFile(filepath.Join(work, "later.txt"), []byte("x"), 0o644)
	git(t, work, "add", "-A")
	git(t, work, "commit", "-q", "-m", "not pushed yet")
	if out, err := makeIn(work, "check-pushed"); err == nil || !strings.Contains(out, "not on origin") {
		t.Errorf("an unpushed commit must fail: %v\n%s", err, out)
	}
	git(t, work, "push", "-q")
	if out, err := makeIn(work, "check-pushed"); err != nil {
		t.Errorf("pushed again must pass: %v\n%s", err, out)
	}
}

// The generated files must come from this history: version.go's hash is a commit
// that exists, and the man page source was built with that version.go.
func TestCheckGenerated(t *testing.T) {
	work := releaseRepo(t)
	h := git(t, work, "rev-parse", "--short", "HEAD")
	write := func(goHash, pageHash string) {
		os.WriteFile(filepath.Join(work, "version.go"), []byte("package x\n\nconst (\n    Version = \"0.0.1\"\n    ReleaseHash = \""+goHash+"\"\n)\n"), 0o644)
		os.WriteFile(filepath.Join(work, "rptop.1.md"), []byte("%rptop(1) user manual | version 0.0.1 "+pageHash+"\n"), 0o644)
	}
	write(h, h)
	if out, err := makeIn(work, "check-generated"); err != nil {
		t.Fatalf("matching files must pass: %v\n%s", err, out)
	}
	write("deadbee", "deadbee")
	if out, err := makeIn(work, "check-generated"); err == nil || !strings.Contains(out, "deadbee") {
		t.Errorf("a hash that is not in this history must fail: %v\n%s", err, out)
	}
	write(h, "0123abc")
	if out, err := makeIn(work, "check-generated"); err == nil || !strings.Contains(out, "rptop.1.md") {
		t.Errorf("a man page built with another version.go must fail: %v\n%s", err, out)
	}
}

// The usual editor and scratch files are ignored; the files of the project are not.
func TestGitignore(t *testing.T) {
	ignored := []string{"x.tmp", "release_notes.tmp", "x.swp", ".x.swp", "x.swo", "x~", "x.bak", "x.orig", ".DS_Store", "dir/y.tmp"}
	kept := []string{"main.go", "README.md", "rptop.1.md", "codemeta.json", "CHANGES.md", "installer.sh", "version.go", "go.sum"}
	for _, f := range ignored {
		if exec.Command("git", "-C", "../..", "check-ignore", "-q", f).Run() != nil {
			t.Errorf("%s is not ignored", f)
		}
	}
	for _, f := range kept {
		if exec.Command("git", "-C", "../..", "check-ignore", "-q", f).Run() == nil {
			t.Errorf("%s is ignored", f)
		}
	}
}
