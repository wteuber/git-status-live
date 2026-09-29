package main

import (
	"errors"
	"fmt"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
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
	Status   *Status // nil for bare and prunable worktrees
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

// loadWorktrees lists the worktrees of the repository in dir and runs git
// status in each one, in parallel.
func loadWorktrees(dir string, untracked bool) Worktrees {
	out, err := exec.Command("git", "-C", dir, "worktree", "list", "--porcelain").Output()
	if err != nil {
		return Worktrees{Err: gitError(err)}
	}
	list := parseWorktrees(out)
	var wg sync.WaitGroup
	sem := make(chan struct{}, runtime.NumCPU())
	for i := range list {
		if !list[i].Selectable() {
			continue
		}
		wg.Add(1)
		go func() {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()
			st := runStatus(list[i].Path, untracked)
			list[i].Status = &st
		}()
	}
	wg.Wait()
	return Worktrees{List: list}
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
