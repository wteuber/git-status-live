package main

import (
	"strings"
	"testing"
)

var sample = []Entry{
	{X: 'R', Y: ' ', Path: "lib/git_tree_version.rb", OrigPath: "lib/version.rb"},
	{X: 'A', Y: ' ', Path: "test/staged.txt"},
	{X: '?', Y: '?', Path: "test/untracked.txt"},
	{X: 'D', Y: ' ', Path: "DELETEME.txt"},
	{X: ' ', Y: 'M', Path: "README.md"},
	{X: 'M', Y: 'M', Path: "both.go"},
}

func plain(lines []string) string {
	return stripANSI(strings.Join(lines, "\n"))
}

func TestRenderTree(t *testing.T) {
	want := `.
├── lib
│   └── version.rb -> git_tree_version.rb (R+)
├── test
│   ├── staged.txt (A+)
│   └── untracked.txt (?)
├── DELETEME.txt (D+)
├── README.md (M)
└── both.go (M+M)`
	if got := plain(renderTree(sample)); got != want {
		t.Errorf("got\n%s\nwant\n%s", got, want)
	}
}

func TestRenderTreeMovedAcrossDirs(t *testing.T) {
	got := plain(renderTree([]Entry{{X: 'R', Y: ' ', Path: "b/x.go", OrigPath: "a/x.go"}}))
	want := ".\n└── a\n    └── x.go -> b/x.go (R+)"
	if got != want {
		t.Errorf("got\n%s\nwant\n%s", got, want)
	}
}

func TestTreeColors(t *testing.T) {
	lines := renderTree(sample)
	for _, tc := range []struct {
		line int
		want string
	}{
		{1, "── " + blue("lib")},
		{2, green("version.rb -> git_tree_version.rb (R+)")},
		{5, red("untracked.txt (?)")},
		{7, red("README.md (M)")},
		{8, red("both.go (") + green("M+") + red("M)")},
	} {
		if !strings.HasSuffix(lines[tc.line], tc.want) {
			t.Errorf("line %d = %q, want suffix %q", tc.line, lines[tc.line], tc.want)
		}
	}
}

func TestRenderList(t *testing.T) {
	want := `Changes to be committed:
        renamed:    lib/version.rb -> lib/git_tree_version.rb
        new file:   test/staged.txt
        deleted:    DELETEME.txt
        modified:   both.go

Changes not staged for commit:
        modified:   README.md
        modified:   both.go

Untracked files:
        test/untracked.txt`
	if got := plain(renderList(sample)); got != want {
		t.Errorf("got\n%s\nwant\n%s", got, want)
	}
}

func TestClean(t *testing.T) {
	for _, lines := range [][]string{renderList(nil), renderTree(nil)} {
		if got := plain(lines); got != "nothing to commit, working tree clean" {
			t.Errorf("got %q", got)
		}
	}
}

func TestTruncate(t *testing.T) {
	s := red("héllo") + " world"
	if got := stripANSI(truncate(s, 4)); got != "héll" {
		t.Errorf("truncate = %q", got)
	}
	if got := truncate("short", 10); got != "short" {
		t.Errorf("truncate = %q", got)
	}
}

var conflicts = []Entry{
	{X: 'U', Y: 'U', Path: "src/both.go"},
	{X: 'A', Y: 'A', Path: "added.go"},
	{X: 'D', Y: 'U', Path: "gone.go"},
}

func TestConflictsInList(t *testing.T) {
	want := `Unmerged paths:
        both modified:   src/both.go
        both added:      added.go
        deleted by us:   gone.go`
	if got := plain(renderList(conflicts)); got != want {
		t.Errorf("got\n%s\nwant\n%s", got, want)
	}
	if got := counts(conflicts); got != "3 conflicts" {
		t.Errorf("counts = %q", got)
	}
}

func TestConflictsInTree(t *testing.T) {
	lines := renderTree(conflicts)
	want := `.
├── src
│   └── both.go (UU)
├── added.go (AA)
└── gone.go (DU)`
	if got := plain(lines); got != want {
		t.Errorf("got\n%s\nwant\n%s", got, want)
	}
	if !strings.HasSuffix(lines[2], red("both.go (UU)")) {
		t.Errorf("conflict is not red: %q", lines[2])
	}
}

func TestTypeChangeAndUnknownCodes(t *testing.T) {
	entries := []Entry{
		{X: 'T', Y: ' ', Path: "link"},
		{X: ' ', Y: 'X', Path: "future"}, // a code git might add one day
	}
	want := `Changes to be committed:
        typechange: link

Changes not staged for commit:
        X:          future`
	if got := plain(renderList(entries)); got != want {
		t.Errorf("got\n%s\nwant\n%s", got, want)
	}
	if got := plain(renderTree(entries)); got != ".\n├── future (X)\n└── link (T+)" {
		t.Errorf("tree = %q", got)
	}
}

func TestStripANSIEdgeCases(t *testing.T) {
	for in, want := range map[string]string{
		"a\x1b[1;34mb\x1b[0m": "ab",
		"a\x1bb":              "ab", // Escape not followed by [ is dropped alone
		"a\x1b[31":            "a",  // unterminated sequence at the end
	} {
		if got := stripANSI(in); got != want {
			t.Errorf("stripANSI(%q) = %q, want %q", in, got, want)
		}
	}
	if got := pad("toolong", 3); got != "toolong" {
		t.Errorf("pad shortened its input: %q", got)
	}
}
