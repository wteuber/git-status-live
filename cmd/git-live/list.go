package main

import "fmt"

var statusLabels = map[byte]string{
	'M': "modified:",
	'A': "new file:",
	'D': "deleted:",
	'R': "renamed:",
	'C': "copied:",
	'T': "typechange:",
}

var unmergedLabels = map[string]string{
	"DD": "both deleted:",
	"AU": "added by us:",
	"UD": "deleted by them:",
	"UA": "added by them:",
	"DU": "deleted by us:",
	"AA": "both added:",
	"UU": "both modified:",
}

// renderList renders entries grouped into sections, like `git status`.
func renderList(entries []Entry) []string {
	if len(entries) == 0 {
		return []string{"nothing to commit, working tree clean"}
	}
	var staged, unmerged, unstaged, untracked []string
	for _, e := range entries {
		switch {
		case e.Unmerged():
			unmerged = append(unmerged, red(fmt.Sprintf("%-17s%s", unmergedLabels[string([]byte{e.X, e.Y})], e.Path)))
		case e.Untracked():
			untracked = append(untracked, red(e.Path))
		default:
			if e.Staged() {
				staged = append(staged, green(listLine(e.X, e)))
			}
			if e.Unstaged() {
				unstaged = append(unstaged, red(listLine(e.Y, e)))
			}
		}
	}
	var lines []string
	section := func(title string, items []string) {
		if len(items) == 0 {
			return
		}
		if len(lines) > 0 {
			lines = append(lines, "")
		}
		lines = append(lines, title)
		for _, it := range items {
			lines = append(lines, "        "+it)
		}
	}
	section("Changes to be committed:", staged)
	section("Unmerged paths:", unmerged)
	section("Changes not staged for commit:", unstaged)
	section("Untracked files:", untracked)
	return lines
}

func listLine(code byte, e Entry) string {
	label, ok := statusLabels[code]
	if !ok {
		label = string(code) + ":"
	}
	path := e.Path
	if e.OrigPath != "" && (code == 'R' || code == 'C') {
		path = e.OrigPath + " -> " + e.Path
	}
	return fmt.Sprintf("%-12s%s", label, path)
}
