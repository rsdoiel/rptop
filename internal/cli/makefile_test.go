package cli

import (
	"os"
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
