package main

import (
	"errors"
	"fmt"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

// Worktree is one entry of `git worktree list --porcelain`.
type Worktree struct {
	Path     string
	Head     string // commit hash, empty for a bare repository
	Branch   string // short branch name, empty if detached or bare
	Detached bool
	Bare     bool
	Locked   bool
	Prunable bool    // its directory is gone; `git worktree prune` removes it
	Status   *Status // nil while loading, and for bare and prunable worktrees
}

// Name is how a worktree is shown: the name of its directory.
func (w Worktree) Name() string { return filepath.Base(w.Path) }

// Selectable reports whether git status can run in the worktree.
func (w Worktree) Selectable() bool { return !w.Bare && !w.Prunable }

// Worktrees is the result of listing all worktrees of a repository.
type Worktrees struct {
	List []Worktree
	Err  error
}

// pollWorktrees loads the worktrees of the repository in dir() repeatedly,
// waiting interval between loads, and sends every update to results. A
// send on kick skips the wait; closing done stops it.
func pollWorktrees(dir func() string, untracked bool, interval time.Duration, results chan<- Worktrees, kick <-chan struct{}, done <-chan struct{}) {
	send := func(wts Worktrees) bool {
		select {
		case results <- wts:
			return true
		case <-done:
			return false
		}
	}
	var prev []Worktree
	for {
		var ok bool
		if prev, ok = loadWorktrees(dir(), untracked, prev, send); !ok {
			return
		}
		select {
		case <-time.After(interval):
		case <-kick:
		case <-done:
			return
		}
	}
}

// loadWorktrees lists the worktrees of the repository in dir and runs git
// status in each one, in parallel. It sends the list as soon as git lists
// it, and again whenever a status arrives, so a slow worktree doesn't hold
// up the others. Until its new status arrives, a worktree keeps its status
// from prev, the previous list. loadWorktrees returns the complete list,
// or false if a send failed.
func loadWorktrees(dir string, untracked bool, prev []Worktree, send func(Worktrees) bool) ([]Worktree, bool) {
	out, err := exec.Command("git", "-C", dir, "worktree", "list", "--porcelain").Output()
	if err != nil {
		return nil, send(Worktrees{Err: gitError(err)})
	}
	list := parseWorktrees(out)
	old := map[string]*Status{}
	for _, w := range prev {
		old[w.Path] = w.Status
	}
	for i := range list {
		if list[i].Selectable() {
			list[i].Status = old[list[i].Path]
		}
	}
	// Each update gets its own copy, since list keeps changing.
	snapshot := func() Worktrees { return Worktrees{List: append([]Worktree(nil), list...)} }
	if !send(snapshot()) {
		return nil, false
	}

	type result struct {
		i  int
		st Status
	}
	var todo []int
	for i, w := range list {
		if w.Selectable() {
			todo = append(todo, i)
		}
	}
	// Buffered, so no goroutine is left blocked if a send fails.
	results := make(chan result, len(todo))
	sem := make(chan struct{}, runtime.NumCPU())
	for _, i := range todo {
		go func() {
			sem <- struct{}{}
			defer func() { <-sem }()
			results <- result{i, runStatus(list[i].Path, untracked)}
		}()
	}
	for range todo {
		r := <-results
		list[r.i].Status = &r.st
		if !send(snapshot()) {
			return nil, false
		}
	}
	return list, true
}

// gitError turns the error of a git command run with Output into a message:
// git's own error output if there is any.
func gitError(err error) error {
	var ee *exec.ExitError
	if !errors.As(err, &ee) {
		return fmt.Errorf("could not run git: %v", err)
	}
	if msg := strings.TrimSpace(string(ee.Stderr)); msg != "" {
		return errors.New(msg)
	}
	return err
}

// parseWorktrees parses the output of `git worktree list --porcelain`:
// one block of "key value" lines per worktree, separated by empty lines.
func parseWorktrees(out []byte) []Worktree {
	var list []Worktree
	var w *Worktree
	for _, line := range strings.Split(string(out), "\n") {
		k, v, _ := strings.Cut(line, " ")
		if k == "worktree" {
			list = append(list, Worktree{Path: v})
			w = &list[len(list)-1]
			continue
		}
		if w == nil {
			continue
		}
		switch k {
		case "HEAD":
			w.Head = v
		case "branch":
			w.Branch = strings.TrimPrefix(v, "refs/heads/")
		case "detached":
			w.Detached = true
		case "bare":
			w.Bare = true
		case "locked":
			w.Locked = true
		case "prunable":
			w.Prunable = true
		}
	}
	return list
}

// gitRoot returns the top-level directory of the worktree that contains
// dir, or "" if dir is not in a worktree.
func gitRoot(dir string) string {
	out, err := exec.Command("git", "-C", dir, "rev-parse", "--show-toplevel").Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}
