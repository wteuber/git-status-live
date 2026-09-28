package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"runtime"
	"sort"
	"strings"
	"testing"
)

func TestParsePorcelain(t *testing.T) {
	out := strings.Join([]string{
		"## main...origin/main [ahead 1, behind 2]",
		"M  staged.go",
		" M unstaged.go",
		"MM both.go",
		"R  new name.go", "old name.go",
		"?? untracked dir/",
		"UU conflict.go",
		"",
	}, "\x00")
	branch, entries := parsePorcelain([]byte(out))
	if branch != "main...origin/main [ahead 1, behind 2]" {
		t.Errorf("branch = %q", branch)
	}
	want := []Entry{
		{X: 'M', Y: ' ', Path: "staged.go"},
		{X: ' ', Y: 'M', Path: "unstaged.go"},
		{X: 'M', Y: 'M', Path: "both.go"},
		{X: 'R', Y: ' ', Path: "new name.go", OrigPath: "old name.go"},
		{X: '?', Y: '?', Path: "untracked dir/"},
		{X: 'U', Y: 'U', Path: "conflict.go"},
	}
	if !reflect.DeepEqual(entries, want) {
		t.Errorf("entries =\n%+v\nwant\n%+v", entries, want)
	}
}

func TestEntryClassification(t *testing.T) {
	for _, tc := range []struct {
		xy                                    string
		staged, unstaged, untracked, unmerged bool
	}{
		{"M ", true, false, false, false},
		{" M", false, true, false, false},
		{"MM", true, true, false, false},
		{"??", false, false, true, false},
		{"UU", false, false, false, true},
		{"AA", false, false, false, true},
	} {
		e := Entry{X: tc.xy[0], Y: tc.xy[1]}
		if e.Staged() != tc.staged || e.Unstaged() != tc.unstaged || e.Untracked() != tc.untracked || e.Unmerged() != tc.unmerged {
			t.Errorf("%q: staged=%v unstaged=%v untracked=%v unmerged=%v", tc.xy, e.Staged(), e.Unstaged(), e.Untracked(), e.Unmerged())
		}
	}
}

func TestMain(m *testing.M) {
	// TestRunStatusGitFailures runs this test binary as a fake git.
	if os.Getenv("GIT_LIVE_FAKE_GIT") == "silent-failure" {
		os.Exit(1)
	}
	os.Exit(m.Run())
}

// testRepo creates a git repository with one commit in a temporary directory,
// isolated from the user's git config. It returns the directory and helpers
// to run git and write files in it.
func testRepo(t *testing.T) (dir string, git func(args ...string), write func(name, content string)) {
	t.Helper()
	// Keep the user's own git config from changing the output. An empty file
	// works on every OS, unlike os.DevNull ("NUL" on Windows).
	emptyConfig := filepath.Join(t.TempDir(), "gitconfig")
	if err := os.WriteFile(emptyConfig, nil, 0o644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("GIT_CONFIG_GLOBAL", emptyConfig)
	t.Setenv("GIT_CONFIG_NOSYSTEM", "1")
	dir = t.TempDir()
	git = func(args ...string) {
		t.Helper()
		cmd := exec.Command("git", append([]string{"-C", dir}, args...)...)
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
	write = func(name, content string) {
		t.Helper()
		path := filepath.Join(dir, name)
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	git("init", "-q", "-b", "main")
	write("old name.txt", "hello\n")
	write("changed.txt", "one\n")
	git("add", ".")
	git("-c", "user.name=test", "-c", "user.email=test@example.com", "commit", "-qm", "init")
	return dir, git, write
}

// TestRunStatus runs the real git binary against a temporary repository.
func TestRunStatus(t *testing.T) {
	dir, git, write := testRepo(t)
	git("mv", "old name.txt", "new name.txt")
	write("changed.txt", "two\n")
	write("untracked.txt", "")
	write(filepath.Join("newdir", "a.txt"), "")
	write(filepath.Join("newdir", "sub", "b.txt"), "")

	st := runStatus(dir, false)
	if st.Err != nil {
		t.Fatal(st.Err)
	}
	if st.Branch != "main" {
		t.Errorf("branch = %q, want main", st.Branch)
	}
	want := []Entry{
		{X: ' ', Y: 'M', Path: "changed.txt"},
		{X: 'R', Y: ' ', Path: "new name.txt", OrigPath: "old name.txt"},
		{X: '?', Y: '?', Path: "newdir/"},
		{X: '?', Y: '?', Path: "untracked.txt"},
	}
	sortEntries(st.Entries)
	if !reflect.DeepEqual(st.Entries, want) {
		t.Errorf("entries =\n%+v\nwant\n%+v", st.Entries, want)
	}

	// With untracked, files in new directories are listed one by one.
	st = runStatus(dir, true)
	if st.Err != nil {
		t.Fatal(st.Err)
	}
	want = []Entry{
		{X: ' ', Y: 'M', Path: "changed.txt"},
		{X: 'R', Y: ' ', Path: "new name.txt", OrigPath: "old name.txt"},
		{X: '?', Y: '?', Path: "newdir/a.txt"},
		{X: '?', Y: '?', Path: "newdir/sub/b.txt"},
		{X: '?', Y: '?', Path: "untracked.txt"},
	}
	sortEntries(st.Entries)
	if !reflect.DeepEqual(st.Entries, want) {
		t.Errorf("untracked entries =\n%+v\nwant\n%+v", st.Entries, want)
	}

	// Stop git from finding a repository above the temp dir.
	notRepo := t.TempDir()
	t.Setenv("GIT_CEILING_DIRECTORIES", filepath.Dir(notRepo))
	if st := runStatus(notRepo, false); st.Err == nil || !strings.Contains(st.Err.Error(), "not a git repository") {
		t.Errorf("non-repo error = %v", st.Err)
	}
}

func sortEntries(entries []Entry) {
	sort.Slice(entries, func(i, j int) bool { return entries[i].Path < entries[j].Path })
}

func TestRunStatusGitFailures(t *testing.T) {
	// No git on the PATH at all.
	t.Setenv("PATH", t.TempDir())
	if st := runStatus(".", false); st.Err == nil || !strings.HasPrefix(st.Err.Error(), "could not run git:") {
		t.Errorf("missing git error = %v", st.Err)
	}

	// A git that fails without printing anything: this test binary, renamed,
	// exits 1 when GIT_LIVE_FAKE_GIT is set (see TestMain).
	bin := t.TempDir()
	name := "git"
	if runtime.GOOS == "windows" {
		name += ".exe"
	}
	self, err := os.ReadFile(os.Args[0])
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(bin, name), self, 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", bin)
	t.Setenv("GIT_LIVE_FAKE_GIT", "silent-failure")
	if st := runStatus(".", false); st.Err == nil || st.Err.Error() != "exit status 1" {
		t.Errorf("silent git failure error = %v, want exit status 1", st.Err)
	}
}
