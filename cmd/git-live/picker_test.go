package main

import (
	"errors"
	"fmt"
	"reflect"
	"strings"
	"testing"
)

// worktreeFixture is a repository with one worktree of each kind. The app
// watches the first one.
func worktreeFixture() Worktrees {
	return Worktrees{List: []Worktree{
		{Path: "/src/repo", Branch: "main", Status: &Status{}},
		{Path: "/src/repo-agent", Branch: "claude/fix-login", Locked: true,
			Status: &Status{Entries: []Entry{{X: ' ', Y: 'M', Path: "a.go"}, {X: '?', Y: '?', Path: "b.go"}}}},
		{Path: "/src/repo-staged", Branch: "feat", Status: &Status{Entries: []Entry{{X: 'A', Y: ' ', Path: "c.go"}}}},
		{Path: "/src/repo-review", Head: "0123456789abcdef", Detached: true, Status: &Status{Err: errors.New("fatal: bad\nmore")}},
		{Path: "/src/repo-gone", Branch: "gone", Prunable: true},
	}}
}

// pickerApp returns an app watching /src/repo with the worktree list open.
func pickerApp(width, height int) *app {
	a := &app{width: width, height: height, dir: "/src/repo",
		status: &Status{Dir: "/src/repo", Root: "/src/repo", Branch: "main"}}
	a.handleKey(char('w'))
	wts := worktreeFixture()
	a.picker.wts = &wts
	return a
}

func TestPickerFrame(t *testing.T) {
	t.Setenv("HOME", "/home/me") // no path in the fixture is in it
	a := pickerApp(100, 9)
	rows := a.frame()
	want := []string{
		" WORKTREES  │  5 worktrees",
		"* repo         main                clean                            /src/repo",
		"  repo-agent   claude/fix-login    1 unstaged, 1 untracked, locked  /src/repo-agent",
		"  repo-staged  feat                1 staged                         /src/repo-staged",
		"  repo-review  (detached 0123456)  error: fatal: bad                /src/repo-review",
		"  repo-gone    gone                missing, prunable                /src/repo-gone",
		"", "",
		" enter switch  ↑↓/jk select  / search  w/esc back  r refresh  q quit",
	}
	for i, w := range want {
		if got := strings.TrimRight(stripANSI(rows[i]), " "); got != w {
			t.Errorf("row %d = %q\n        want %q", i, got, w)
		}
	}
	// The watched worktree is selected when the list opens, in reverse video
	// across the whole width.
	if !strings.HasPrefix(rows[1], ansiReverse) || visibleLen(rows[1]) != 100 {
		t.Errorf("selected row = %q", rows[1])
	}
	for row, want := range map[int]string{
		2: red("1 unstaged, 1 untracked, locked"), // anything unstaged: red
		3: green("1 staged                       "),
		4: red("error: fatal: bad              "),
		5: red("missing, prunable              "),
	} {
		if !strings.Contains(rows[row], want) {
			t.Errorf("row %d = %q, want it to contain %q", row, rows[row], want)
		}
	}
	if !strings.Contains(rows[2], blue("repo-agent ")) || !strings.Contains(rows[2], dim("/src/repo-agent")) {
		t.Errorf("row 2 colors = %q", rows[2])
	}
}

func TestPickerKeys(t *testing.T) {
	a := pickerApp(100, 5) // 3 body rows
	selected := func() string { a.frame(); return a.picker.selected }

	for _, step := range []struct {
		key  key
		want string
	}{
		{char('j'), "/src/repo-agent"},
		{press(keyDown), "/src/repo-staged"},
		{char('k'), "/src/repo-agent"},
		{press(keyUp), "/src/repo"},
		{press(keyUp), "/src/repo"}, // can't move above the first
		{char('G'), "/src/repo-gone"},
		{press(keyDown), "/src/repo-gone"}, // or below the last
		{char('g'), "/src/repo"},
		{press(keyEnd), "/src/repo-gone"},
		{press(keyHome), "/src/repo"},
		{press(keyPageDown), "/src/repo-review"}, // 3 rows down
		{char(' '), "/src/repo-gone"},
		{press(keyPageUp), "/src/repo-agent"},
		{char('t'), "/src/repo-agent"}, // t doesn't toggle the view here
	} {
		a.handleKey(step.key)
		if got := selected(); got != step.want {
			t.Fatalf("after key %+v selected %q, want %q", step.key, got, step.want)
		}
	}
	if a.view != listView {
		t.Error("t toggled the view behind the worktree list")
	}
	if quit, refresh := a.handleKey(char('r')); quit || !refresh {
		t.Error("r did not ask for a refresh")
	}
	if quit, _ := a.handleKey(char('q')); !quit {
		t.Error("q did not quit")
	}
	if quit, _ := a.handleKey(press(keyQuit)); !quit {
		t.Error("Ctrl-C did not quit")
	}
}

func TestPickerSwitch(t *testing.T) {
	a := pickerApp(100, 10)
	a.scroll = 3
	a.handleKey(char('j'))
	a.handleKey(press(keyEnter))
	if a.picker != nil || a.dir != "/src/repo-agent" || a.status != nil || a.scroll != 0 {
		t.Errorf("after switching: picker %v, dir %q, status %v, scroll %d", a.picker, a.dir, a.status, a.scroll)
	}
	if got := stripANSI(a.frame()[0]); !strings.HasPrefix(got, " LIST ") {
		t.Errorf("header after switching = %q", got)
	}

	// Selecting the worktree that is already watched just closes the list.
	a = pickerApp(100, 10)
	status := a.status
	a.handleKey(press(keyEnter))
	if a.picker != nil || a.dir != "/src/repo" || a.status != status {
		t.Errorf("selecting the watched worktree: picker %v, dir %q, status %v", a.picker, a.dir, a.status)
	}

	// A prunable worktree has no working tree to watch.
	a = pickerApp(100, 10)
	a.handleKey(char('G'))
	a.handleKey(press(keyEnter))
	if a.picker == nil || a.dir != "/src/repo" {
		t.Errorf("selecting a prunable worktree: picker %v, dir %q", a.picker, a.dir)
	}
}

func TestPickerClose(t *testing.T) {
	for _, k := range []key{press(keyEsc), char('w'), char('W')} {
		a := pickerApp(100, 10)
		a.handleKey(char('j'))
		a.handleKey(k)
		if a.picker != nil || a.dir != "/src/repo" {
			t.Errorf("key %+v: picker %v, dir %q", k, a.picker, a.dir)
		}
	}
	// W opens it too.
	a := &app{width: 80, height: 10}
	if a.handleKey(char('W')); a.picker == nil {
		t.Error("W did not open the worktree list")
	}
}

func TestPickerKeepsSelectionWhenListChanges(t *testing.T) {
	a := pickerApp(100, 10)
	a.handleKey(char('j')) // repo-agent
	// A new worktree sorts before it, as git lists linked worktrees by path.
	wts := worktreeFixture()
	wts.List = append(wts.List[:1], append([]Worktree{{Path: "/src/repo-aaa", Branch: "x", Status: &Status{}}}, wts.List[1:]...)...)
	a.picker.wts = &wts
	if _, sel := a.body(); sel != 2 || a.picker.selected != "/src/repo-agent" {
		t.Errorf("selected line %d (%q), want 2 (repo-agent)", sel, a.picker.selected)
	}
	// If the selected worktree is removed, the first one is selected.
	wts = Worktrees{List: wts.List[:1]}
	a.picker.wts = &wts
	if _, sel := a.body(); sel != 0 {
		t.Errorf("selected line %d after removal, want 0", sel)
	}
	a.handleKey(char('j'))
	if a.picker.selected != "/src/repo" {
		t.Errorf("selected %q", a.picker.selected)
	}
}

func TestPickerLoadingAndErrors(t *testing.T) {
	a := &app{width: 60, height: 8, interval: 500 * 1e6}
	a.handleKey(char('w')) // before the first git status: nothing selected
	if a.picker.selected != "" {
		t.Errorf("selected %q before the first status", a.picker.selected)
	}
	rows := a.frame()
	if got := stripANSI(rows[0]); strings.TrimSpace(got) != "WORKTREES" {
		t.Errorf("header while loading = %q", got)
	}
	if got := plain(rows); !strings.Contains(got, "Listing worktrees…") {
		t.Errorf("loading frame:\n%s", got)
	}
	// Keys do nothing harmful while there is nothing to select.
	for _, k := range []key{char('j'), char('G'), press(keyEnter)} {
		a.handleKey(k)
	}
	if a.picker == nil || a.dir != "" {
		t.Errorf("keys without a list: picker %v, dir %q", a.picker, a.dir)
	}

	a.picker.wts = &Worktrees{Err: errors.New("fatal: not a git repository")}
	rows = a.frame()
	if got := strings.TrimSpace(stripANSI(rows[0])); got != "WORKTREES" {
		t.Errorf("header on error = %q", got)
	}
	if got := stripANSI(rows[1]); got != "fatal: not a git repository" || !strings.HasPrefix(rows[1], ansiRed) {
		t.Errorf("error row = %q", rows[1])
	}
	if got := stripANSI(rows[3]); got != "Retrying every 500ms…" {
		t.Errorf("retry row = %q", got)
	}

	a.picker.wts = &Worktrees{List: []Worktree{{Path: "/only", Branch: "main"}}}
	if got := stripANSI(a.frame()[0]); strings.TrimSpace(got) != "WORKTREES  │  1 worktree" {
		t.Errorf("header with one worktree = %q", got)
	}
}

func TestPickerScrollsToSelection(t *testing.T) {
	var list []Worktree
	for i := range 20 {
		list = append(list, Worktree{Path: fmt.Sprintf("/wt/%02d", i), Branch: "b", Status: &Status{}})
	}
	a := &app{width: 60, height: 7, dir: "/wt/00", status: &Status{Dir: "/wt/00", Root: "/wt/00"}}
	a.handleKey(char('w'))
	a.picker.wts = &Worktrees{List: list}
	firstRow := func() string { return strings.Fields(stripANSI(a.frame()[1]))[0] }

	if got := firstRow(); got != "*" { // the watched worktree, marked
		t.Fatalf("first row starts with %q", got)
	}
	a.handleKey(char('G'))
	rows := a.frame()
	if got := strings.Fields(stripANSI(rows[5]))[0]; got != "19" || !strings.HasPrefix(rows[5], ansiReverse) {
		t.Errorf("last row = %q, want the selected worktree 19", stripANSI(rows[5]))
	}
	if footer := stripANSI(rows[6]); !strings.HasSuffix(footer, "16-20/20 ") {
		t.Errorf("footer = %q", footer)
	}
	for range 6 {
		a.handleKey(char('k')) // 13: above the visible rows
	}
	if got := firstRow(); got != "13" {
		t.Errorf("first row after moving up = %q, want 13", got)
	}
	// Scrolling the list view is separate from the worktree list.
	if a.scroll != 0 {
		t.Errorf("list view scroll = %d", a.scroll)
	}
}

func TestWorktreeLabels(t *testing.T) {
	for _, tc := range []struct {
		w       Worktree
		branch  string
		summary string
		color   string
	}{
		{Worktree{Branch: "main", Status: &Status{}}, "main", "clean", ansiDim},
		{Worktree{Branch: "main"}, "main", "loading…", ansiDim}, // status not known yet
		{Worktree{Bare: true}, "(bare)", "no working tree", ansiDim},
		{Worktree{Detached: true, Head: "abc"}, "(detached abc)", "loading…", ansiDim},
		{Worktree{Branch: "x", Prunable: true, Locked: true}, "x", "missing, prunable, locked", ansiRed},
		{Worktree{Branch: "x", Status: &Status{Entries: conflicts}}, "x", "3 conflicts", ansiRed},
		{Worktree{Branch: "x", Status: &Status{Entries: []Entry{{X: 'M', Y: 'M'}}}}, "x", "1 staged, 1 unstaged", ansiRed},
	} {
		if got := branchLabel(tc.w); got != tc.branch {
			t.Errorf("branchLabel(%+v) = %q, want %q", tc.w, got, tc.branch)
		}
		if got := summary(tc.w); got != tc.summary {
			t.Errorf("summary(%+v) = %q, want %q", tc.w, got, tc.summary)
		}
		if got := summaryColor(tc.w)("s"); !strings.HasPrefix(got, tc.color) {
			t.Errorf("summaryColor(%+v) = %q, want %q", tc.w, got, tc.color)
		}
	}
}

func TestHomePath(t *testing.T) {
	t.Setenv("HOME", "/home/me")
	t.Setenv("USERPROFILE", "/home/me") // os.UserHomeDir on Windows
	for path, want := range map[string]string{
		"/home/me/src/repo": "~/src/repo",
		"/home/me":          "~",
		"/home/meet/repo":   "/home/meet/repo", // not in the home directory
		"/src/repo":         "/src/repo",
	} {
		if got := homePath(path); got != want {
			t.Errorf("homePath(%q) = %q, want %q", path, got, want)
		}
	}
	t.Setenv("HOME", "")
	t.Setenv("USERPROFILE", "")
	if got := homePath("/home/me/src"); got != "/home/me/src" {
		t.Errorf("without a home directory: %q", got)
	}
}

func TestPickerSearch(t *testing.T) {
	a := pickerApp(100, 9)
	header := func() string { return strings.TrimSpace(stripANSI(a.frame()[0])) }
	names := func() []string {
		items, _ := a.picker.items()
		var got []string
		for _, w := range items {
			got = append(got, w.Name())
		}
		return got
	}
	typeText := func(s string) {
		for _, r := range s {
			a.handleKey(char(r))
		}
	}

	a.handleKey(char('/'))
	if got := header(); got != "WORKTREES  │  5 worktrees  │  /▏" {
		t.Errorf("header when the search starts = %q", got)
	}
	if footer := stripANSI(a.frame()[8]); !strings.HasPrefix(footer, " type to search") {
		t.Errorf("footer while searching = %q", footer)
	}

	// Keys that do something else without a search are text now.
	typeText("Q")
	if a.picker == nil || a.picker.query != "Q" {
		t.Fatalf("q while searching: picker %v", a.picker)
	}
	if got := names(); got != nil {
		t.Errorf("Q matches %v", got)
	}
	a.handleKey(press(keyBackspace))

	// Words match the name, branch or path, in any case and order.
	for _, tc := range []struct {
		query string
		want  []string
	}{
		{"agent", []string{"repo-agent"}},                              // name
		{"CLAUDE/", []string{"repo-agent"}},                            // branch, any case
		{"detached", []string{"repo-review"}},                          // detached label
		{"/src/repo-s", []string{"repo-staged"}},                       // path
		{"repo g", []string{"repo-agent", "repo-staged", "repo-gone"}}, // g: agent, staged, gone
		{"login claude", []string{"repo-agent"}},                       // every word, any order
		{"  feat  ", []string{"repo-staged"}},                          // extra spaces
		{"nothing", nil},
	} {
		a.picker.query = ""
		typeText(tc.query)
		if got := names(); !reflect.DeepEqual(got, tc.want) {
			t.Errorf("search %q = %v, want %v", tc.query, got, tc.want)
		}
	}
	// No match: a message, and the header counts what's left.
	if got := plain(a.frame()[1:2]); got != `No worktree matches "nothing".` {
		t.Errorf("no match = %q", got)
	}
	if got := header(); got != "WORKTREES  │  0 of 5 worktrees  │  /nothing▏" {
		t.Errorf("header without matches = %q", got)
	}
	a.handleKey(press(keyEnter)) // nothing to switch to
	if a.picker == nil || a.dir != "/src/repo" {
		t.Errorf("Enter without matches: picker %v, dir %q", a.picker, a.dir)
	}

	// Backspace edits the search. The first match is selected when the
	// selected worktree doesn't match.
	a.picker.query = ""
	typeText("repo-s")
	if got := header(); got != "WORKTREES  │  1 of 5 worktrees  │  /repo-s▏" {
		t.Errorf("header = %q", got)
	}
	a.handleKey(press(keyBackspace))
	a.handleKey(press(keyBackspace))
	if a.picker.query != "repo" || len(names()) != 5 {
		t.Errorf("after two backspaces: query %q, %d matches", a.picker.query, len(names()))
	}

	// Arrows select among the matches, and Enter switches.
	a.picker.query = ""
	typeText("repo-")
	a.handleKey(press(keyDown))
	a.handleKey(press(keyDown))
	a.handleKey(press(keyUp))
	a.handleKey(press(keyEnter))
	if a.picker != nil || a.dir != "/src/repo-staged" {
		t.Errorf("after selecting a match: picker %v, dir %q", a.picker, a.dir)
	}
}

func TestPickerSearchCancel(t *testing.T) {
	a := pickerApp(100, 9)
	a.handleKey(char('/'))
	a.handleKey(char('a'))
	// Escape clears the search, and a second one closes the list.
	a.handleKey(press(keyEsc))
	if a.picker == nil || a.picker.searching || a.picker.query != "" {
		t.Fatalf("after Escape: %+v", a.picker)
	}
	if got := strings.TrimSpace(stripANSI(a.frame()[0])); got != "WORKTREES  │  5 worktrees" {
		t.Errorf("header after Escape = %q", got)
	}
	a.handleKey(press(keyEsc))
	if a.picker != nil {
		t.Error("the second Escape did not close the list")
	}

	// Backspace on an empty search ends it.
	a = pickerApp(100, 9)
	a.handleKey(char('/'))
	a.handleKey(press(keyBackspace))
	if a.picker.searching {
		t.Error("Backspace on an empty search didn't end it")
	}

	// Multi-byte characters are deleted whole.
	a.handleKey(char('/'))
	a.handleKey(char('ü'))
	a.handleKey(char('x'))
	a.handleKey(press(keyBackspace))
	if a.picker.query != "ü" {
		t.Errorf("query = %q, want ü", a.picker.query)
	}
	a.handleKey(press(keyBackspace))
	if a.picker.query != "" || !a.picker.searching {
		t.Errorf("query = %q, searching %v", a.picker.query, a.picker.searching)
	}

	// Ctrl-C quits while searching, and Tab is ignored.
	a.handleKey(press(keyTab))
	if !a.picker.searching || a.picker.query != "" {
		t.Errorf("Tab changed the search: %+v", a.picker)
	}
	if quit, _ := a.handleKey(press(keyQuit)); !quit {
		t.Error("Ctrl-C did not quit while searching")
	}
}
