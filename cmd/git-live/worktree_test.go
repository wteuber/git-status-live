package main

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestParseWorktrees(t *testing.T) {
	out := `worktree /src/repo
HEAD 1111111111111111111111111111111111111111
branch refs/heads/main

worktree /src/repo-detached
HEAD 2222222222222222222222222222222222222222
detached

worktree /src/repo-agent
HEAD 3333333333333333333333333333333333333333
branch refs/heads/claude/fix-login
locked agent busy

worktree /src/repo-gone
HEAD 4444444444444444444444444444444444444444
branch refs/heads/gone
locked
prunable gitdir file points to non-existent location

`
	want := []Worktree{
		{Path: "/src/repo", Head: "1111111111111111111111111111111111111111", Branch: "main"},
		{Path: "/src/repo-detached", Head: "2222222222222222222222222222222222222222", Detached: true},
		{Path: "/src/repo-agent", Head: "3333333333333333333333333333333333333333", Branch: "claude/fix-login", Locked: true},
		{Path: "/src/repo-gone", Head: "4444444444444444444444444444444444444444", Branch: "gone", Locked: true, Prunable: true},
	}
	if got := parseWorktrees([]byte(out)); !reflect.DeepEqual(got, want) {
		t.Errorf("parseWorktrees =\n%+v\nwant\n%+v", got, want)
	}

	bare := parseWorktrees([]byte("worktree /src/repo.git\nbare\n\n"))
	if len(bare) != 1 || !bare[0].Bare || bare[0].Selectable() {
		t.Errorf("bare = %+v, want one bare worktree that can't be selected", bare)
	}
	// Lines before the first worktree, and unknown keys, are ignored.
	if got := parseWorktrees([]byte("junk\nworktree /a\nfuture x\n")); !reflect.DeepEqual(got, []Worktree{{Path: "/a"}}) {
		t.Errorf("unexpected lines: %+v", got)
	}
	if got := parseWorktrees(nil); got != nil {
		t.Errorf("no output = %+v, want nil", got)
	}
}

func TestWorktreeName(t *testing.T) {
	for path, want := range map[string]string{
		"/src/repo-agent":           "repo-agent",
		"/src/repo-agent/":          "repo-agent",
		"C:/Users/me/src/repo-feat": "repo-feat", // git's paths on Windows
	} {
		if got := (Worktree{Path: path}).Name(); got != want {
			t.Errorf("Name(%q) = %q, want %q", path, got, want)
		}
	}
}

// TestLoadWorktrees runs the real git binary against a repository with one
// worktree of each kind.
func TestLoadWorktrees(t *testing.T) {
	dir, git, write := testRepo(t)
	parent := t.TempDir()
	feat := filepath.Join(parent, "feat")
	git("worktree", "add", "-q", "-b", "feat", feat)
	git("worktree", "add", "-q", "--detach", filepath.Join(parent, "detached"))
	git("worktree", "add", "-q", "-b", "agent", filepath.Join(parent, "agent"))
	git("worktree", "lock", "--reason", "agent busy", filepath.Join(parent, "agent"))
	gone := filepath.Join(parent, "gone")
	git("worktree", "add", "-q", "-b", "gone", gone)
	if err := os.RemoveAll(gone); err != nil {
		t.Fatal(err)
	}
	write("changed.txt", "two\n") // in the main worktree
	if err := os.WriteFile(filepath.Join(feat, "new.txt"), nil, 0o644); err != nil {
		t.Fatal(err)
	}

	wts := loadWorktrees(dir, false)
	if wts.Err != nil {
		t.Fatal(wts.Err)
	}
	byName := map[string]Worktree{}
	for _, w := range wts.List {
		byName[w.Name()] = w
	}
	if len(wts.List) != 5 || wts.List[0].Path != gitRoot(dir) {
		t.Fatalf("worktrees = %+v, want 5 with the main worktree first", wts.List)
	}
	if w := byName["feat"]; w.Path != gitRoot(feat) || w.Branch != "feat" || counts(w.Status.Entries) != "1 untracked" {
		t.Errorf("feat = %+v, status %+v", w, w.Status)
	}
	if w := byName["detached"]; !w.Detached || w.Branch != "" || w.Status.Err != nil || len(w.Head) != 40 {
		t.Errorf("detached = %+v", w)
	}
	if w := byName["agent"]; !w.Locked || !w.Selectable() || w.Status == nil {
		t.Errorf("locked = %+v; want it listed with a status", w)
	}
	if w := byName["gone"]; !w.Prunable || w.Selectable() || w.Status != nil {
		t.Errorf("prunable = %+v; want it without a status", w)
	}
	if st := wts.List[0].Status; counts(st.Entries) != "1 unstaged" {
		t.Errorf("main worktree status = %+v", st)
	}

	// Listing from a linked worktree gives the same list.
	if again := loadWorktrees(feat, false); len(again.List) != 5 || again.List[0].Path != wts.List[0].Path {
		t.Errorf("from a linked worktree: %+v", again)
	}

	// With untracked, statuses list the files in new directories.
	if err := os.MkdirAll(filepath.Join(feat, "newdir"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(feat, "newdir", "a.txt"), nil, 0o644); err != nil {
		t.Fatal(err)
	}
	for _, w := range loadWorktrees(dir, true).List {
		if w.Name() == "feat" && counts(w.Status.Entries) != "2 untracked" {
			t.Errorf("untracked feat status = %q", counts(w.Status.Entries))
		}
	}
}

func TestLoadWorktreesBare(t *testing.T) {
	dir, git, _ := testRepo(t)
	bare := filepath.Join(t.TempDir(), "repo.git")
	git("clone", "-q", "--bare", dir, bare)
	wts := loadWorktrees(bare, false)
	if wts.Err != nil || len(wts.List) != 1 || !wts.List[0].Bare || wts.List[0].Status != nil {
		t.Errorf("bare repository = %+v", wts)
	}
}

func TestLoadWorktreesErrors(t *testing.T) {
	notRepo := t.TempDir()
	t.Setenv("GIT_CEILING_DIRECTORIES", filepath.Dir(notRepo))
	if wts := loadWorktrees(notRepo, false); wts.Err == nil || !strings.Contains(wts.Err.Error(), "not a git repository") {
		t.Errorf("non-repo error = %v", wts.Err)
	}

	t.Setenv("PATH", t.TempDir())
	if wts := loadWorktrees(".", false); wts.Err == nil || !strings.HasPrefix(wts.Err.Error(), "could not run git:") {
		t.Errorf("missing git error = %v", wts.Err)
	}

	silentFailingGit(t)
	if wts := loadWorktrees(".", false); wts.Err == nil || wts.Err.Error() != "exit status 1" {
		t.Errorf("silent git failure error = %v, want exit status 1", wts.Err)
	}
}

func TestGitRoot(t *testing.T) {
	dir, _, write := testRepo(t)
	write(filepath.Join("sub", "x.txt"), "")
	root := gitRoot(dir)
	if root == "" {
		t.Fatal("no root for the repository")
	}
	if got := gitRoot(filepath.Join(dir, "sub")); got != root {
		t.Errorf("root of a subdirectory = %q, want %q", got, root)
	}
	notRepo := t.TempDir()
	t.Setenv("GIT_CEILING_DIRECTORIES", filepath.Dir(notRepo))
	if got := gitRoot(notRepo); got != "" {
		t.Errorf("root outside a repository = %q, want empty", got)
	}
}
