package main

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"
)

// Entry is one path reported by `git status --porcelain`.
// X is the index (staged) status, Y the work tree (unstaged) status.
type Entry struct {
	X, Y     byte
	Path     string
	OrigPath string // source path of a rename or copy
}

// Status is the result of a single `git status` run.
type Status struct {
	Branch  string // branch line without the leading "## "
	Entries []Entry
	Err     error
}

func (e Entry) Untracked() bool { return e.X == '?' && e.Y == '?' }
func (e Entry) Ignored() bool   { return e.X == '!' && e.Y == '!' }

// Unmerged reports whether the entry is in a merge conflict.
func (e Entry) Unmerged() bool {
	switch string([]byte{e.X, e.Y}) {
	case "DD", "AU", "UD", "UA", "DU", "AA", "UU":
		return true
	}
	return false
}

func (e Entry) Staged() bool {
	return !e.Untracked() && !e.Ignored() && !e.Unmerged() && e.X != ' '
}

func (e Entry) Unstaged() bool {
	return !e.Untracked() && !e.Ignored() && !e.Unmerged() && e.Y != ' '
}

// runStatus runs git status in dir and parses the result. With untracked set,
// files inside untracked directories are listed individually
// (--untracked-files=all) instead of as one "dir/" entry.
func runStatus(dir string, untracked bool) Status {
	args := []string{"-C", dir, "status", "--porcelain=v1", "-z", "--branch"}
	if untracked {
		args = append(args, "--untracked-files=all")
	}
	cmd := exec.Command("git", args...)
	// Never take index.lock while polling, so we can't get in the way of
	// git commands the user runs at the same time.
	cmd.Env = append(os.Environ(), "GIT_OPTIONAL_LOCKS=0")
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err := cmd.Run()
	var st Status
	if err != nil {
		msg := strings.TrimSpace(stderr.String())
		if msg == "" {
			msg = err.Error()
		}
		var ee *exec.ExitError
		if !errors.As(err, &ee) {
			msg = fmt.Sprintf("could not run git: %v", err)
		}
		st.Err = errors.New(msg)
		return st
	}
	st.Branch, st.Entries = parsePorcelain(stdout.Bytes())
	return st
}

// gitConfig returns the value of a git config key as seen from dir, or "" if
// it isn't set or git can't read it.
func gitConfig(dir, key string) string {
	out, err := exec.Command("git", "-C", dir, "config", "--get", key).Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}

// parsePorcelain parses the output of `git status --porcelain=v1 -z --branch`.
func parsePorcelain(out []byte) (branch string, entries []Entry) {
	fields := strings.Split(string(out), "\x00")
	for i := 0; i < len(fields); i++ {
		f := fields[i]
		if strings.HasPrefix(f, "## ") {
			branch = f[3:]
			continue
		}
		if len(f) < 4 {
			continue
		}
		e := Entry{X: f[0], Y: f[1], Path: f[3:]}
		// With -z, renames and copies are "XY new\0orig\0".
		if (e.X == 'R' || e.X == 'C' || e.Y == 'R' || e.Y == 'C') && i+1 < len(fields) {
			i++
			e.OrigPath = fields[i]
		}
		entries = append(entries, e)
	}
	return branch, entries
}
