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
