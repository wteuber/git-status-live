package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// picker is the worktree list: it shows every worktree of the repository
// and lets the user switch the live view to another one.
type picker struct {
	wts       *Worktrees // nil until the first list arrives
	selected  string     // path of the selected worktree
	scroll    int
	searching bool   // typing a search
	query     string // shows only the worktrees that match it
}

// openPicker shows the worktree list, with the watched worktree selected.
func (a *app) openPicker() {
	a.picker = &picker{}
	if a.status != nil {
		a.picker.selected = a.status.Root
	}
}

// items returns the worktrees the picker shows, and the index of the
// selected one (0 if the selected worktree isn't listed).
func (p *picker) items() ([]Worktree, int) {
	if p.wts == nil {
		return nil, 0
	}
	var list []Worktree
	for _, w := range p.wts.List {
		if matches(w, p.query) {
			list = append(list, w)
		}
	}
	for i, w := range list {
		if w.Path == p.selected {
			return list, i
		}
	}
	return list, 0
}

// matches reports whether a worktree's name, branch or path contains every
// word of query, ignoring case.
func matches(w Worktree, query string) bool {
	text := strings.ToLower(w.Name() + " " + branchLabel(w) + " " + w.Path)
	for _, word := range strings.Fields(strings.ToLower(query)) {
		if !strings.Contains(text, word) {
			return false
		}
	}
	return true
}

// handlePickerKey applies a key press while the worktree list is shown.
func (a *app) handlePickerKey(k key) (quit, refresh bool) {
	p := a.picker
	if p.searching {
		switch {
		case k.code == keyEsc:
			p.searching, p.query = false, ""
			return false, false
		case k.code == keyBackspace:
			if p.query == "" {
				p.searching = false
			}
			r := []rune(p.query)
			p.query = string(r[:max(len(r)-1, 0)])
			return false, false
		case k.code == keyChar:
			p.query += string(k.r)
			return false, false
		}
		// Enter, Ctrl-C and the cursor keys work as they do without a search.
	} else if k.is('/') {
		p.searching = true
		return false, false
	}
	items, i := p.items()
	switch {
	case k.code == keyQuit, k.is('q'), k.is('Q'):
		return true, false
	case k.is('r'), k.is('R'):
		return false, true
	case k.code == keyEsc, k.is('w'), k.is('W'):
		a.picker = nil
		return false, false
	case k.code == keyEnter:
		if i < len(items) && items[i].Selectable() {
			a.switchTo(items[i].Path)
		}
		return false, false
	case k.code == keyUp, k.is('k'):
		i--
	case k.code == keyDown, k.is('j'):
		i++
	case k.code == keyPageUp:
		i -= a.bodyHeight()
	case k.code == keyPageDown, k.is(' '):
		i += a.bodyHeight()
	case k.code == keyHome, k.is('g'):
		i = 0
	case k.code == keyEnd, k.is('G'):
		i = len(items) - 1
	}
	if len(items) > 0 {
		p.selected = items[min(max(i, 0), len(items)-1)].Path
	}
	return false, false
}

// switchTo closes the worktree list and watches the worktree at path.
func (a *app) switchTo(path string) {
	a.picker = nil
	if a.status != nil && a.status.Root == path {
		return // already watching it
	}
	a.dir = path
	a.status = nil
	a.scroll = 0
}

func (a *app) pickerHeader() string {
	s := " WORKTREES"
	p := a.picker
	if p.wts != nil && p.wts.Err == nil {
		all := plural(len(p.wts.List), "worktree")
		if items, _ := p.items(); p.query != "" {
			all = fmt.Sprintf("%d of %s", len(items), all)
		}
		s += "  │  " + all
	}
	if p.searching {
		s += "  │  /" + p.query + "▏"
	}
	return s
}

func plural(n int, word string) string {
	if n == 1 {
		return fmt.Sprintf("%d %s", n, word)
	}
	return fmt.Sprintf("%d %ss", n, word)
}

// pickerBody returns the lines of the worktree list and which line is
// selected (-1 for none).
func (a *app) pickerBody() ([]string, int) {
	p := a.picker
	switch {
	case p.wts == nil:
		return a.centered("Listing worktrees…"), -1
	case p.wts.Err != nil:
		return a.errorLines(p.wts.Err), -1
	}
	items, sel := p.items()
	if len(items) == 0 && p.query != "" {
		return []string{dim(fmt.Sprintf("No worktree matches %q.", p.query))}, -1
	}
	current := ""
	if a.status != nil {
		current = a.status.Root
	}

	// Columns: name, branch, summary, path.
	type row struct{ mark, name, branch, summary, path string }
	rows := make([]row, len(items))
	var nameW, branchW, summaryW int
	for i, w := range items {
		r := row{mark: "  ", name: w.Name(), branch: branchLabel(w), summary: summary(w), path: homePath(w.Path)}
		if w.Path == current {
			r.mark = "* "
		}
		rows[i] = r
		nameW = max(nameW, visibleLen(r.name))
		branchW = max(branchW, visibleLen(r.branch))
		summaryW = max(summaryW, visibleLen(r.summary))
	}
	lines := make([]string, len(rows))
	for i, r := range rows {
		if i == sel {
			line := r.mark + pad(r.name, nameW) + "  " + pad(r.branch, branchW) + "  " + pad(r.summary, summaryW) + "  " + r.path
			lines[i] = ansiReverse + pad(truncate(line, a.width), a.width) + ansiReset
			continue
		}
		lines[i] = r.mark + blue(pad(r.name, nameW)) + "  " + green(pad(r.branch, branchW)) + "  " +
			summaryColor(items[i])(pad(r.summary, summaryW)) + "  " + dim(r.path)
	}
	return lines, sel
}

// branchLabel is the branch of a worktree, or what it has instead.
func branchLabel(w Worktree) string {
	switch {
	case w.Bare:
		return "(bare)"
	case w.Detached:
		return "(detached " + w.Head[:min(len(w.Head), 7)] + ")"
	}
	return w.Branch
}

// summary sums up the changes in a worktree, like the header does.
func summary(w Worktree) string {
	var s string
	switch {
	case w.Bare:
		s = "no working tree"
	case w.Prunable:
		s = "missing, prunable"
	case w.Status == nil:
		s = "…"
	case w.Status.Err != nil:
		s = "error: " + strings.SplitN(w.Status.Err.Error(), "\n", 2)[0]
	case len(w.Status.Entries) == 0:
		s = "clean"
	default:
		s = counts(w.Status.Entries)
	}
	if w.Locked {
		s += ", locked"
	}
	return s
}

// summaryColor colors a summary like the tree colors files: green if all
// changes are staged, red if anything else changed or failed.
func summaryColor(w Worktree) func(string) string {
	switch {
	case w.Prunable || (w.Status != nil && w.Status.Err != nil):
		return red
	case w.Status == nil || len(w.Status.Entries) == 0:
		return dim
	}
	for _, e := range w.Status.Entries {
		if e.Unstaged() || e.Untracked() || e.Unmerged() {
			return red
		}
	}
	return green
}

// homePath shortens a path in the home directory to start with ~.
func homePath(path string) string {
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		return path
	}
	// git reports paths with forward slashes, also on Windows.
	home = filepath.ToSlash(home)
	if rest, ok := strings.CutPrefix(filepath.ToSlash(path), home); ok && (rest == "" || rest[0] == '/') {
		return "~" + rest
	}
	return path
}
