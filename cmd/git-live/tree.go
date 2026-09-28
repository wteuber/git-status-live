package main

import (
	"path"
	"sort"
	"strings"
)

type treeNode struct {
	name     string
	label    string // text shown instead of name, e.g. "old -> new" for renames
	entry    *Entry
	isDir    bool
	children map[string]*treeNode
}

func (n *treeNode) child(name string, isDir bool) *treeNode {
	if n.children == nil {
		n.children = map[string]*treeNode{}
	}
	key := name
	if isDir {
		key += "/"
	}
	c, ok := n.children[key]
	if !ok {
		c = &treeNode{name: name, isDir: isDir}
		n.children[key] = c
	}
	return c
}

// renderTree renders entries as a tree, like git-status-tree.
func renderTree(entries []Entry) []string {
	if len(entries) == 0 {
		return []string{"nothing to commit, working tree clean"}
	}
	root := &treeNode{name: ".", isDir: true}
	for i := range entries {
		e := &entries[i]
		// Renames are shown at their original location, pointing to the new name.
		p := e.Path
		if e.OrigPath != "" {
			p = e.OrigPath
		}
		// Untracked directories are reported as "dir/" and shown as one node.
		untrackedDir := strings.HasSuffix(p, "/")
		parts := strings.Split(strings.TrimSuffix(p, "/"), "/")
		n := root
		for _, part := range parts[:len(parts)-1] {
			n = n.child(part, true)
		}
		leaf := n.child(parts[len(parts)-1], untrackedDir)
		leaf.entry = e
		if e.OrigPath != "" {
			target := e.Path
			if path.Dir(e.Path) == path.Dir(e.OrigPath) {
				target = path.Base(e.Path)
			}
			leaf.label = leaf.name + " -> " + target
		}
	}
	lines := []string{"."}
	return appendChildren(lines, root, "")
}

func appendChildren(lines []string, n *treeNode, prefix string) []string {
	kids := make([]*treeNode, 0, len(n.children))
	for _, c := range n.children {
		kids = append(kids, c)
	}
	sort.Slice(kids, func(i, j int) bool {
		if kids[i].isDir != kids[j].isDir {
			return kids[i].isDir
		}
		return kids[i].name < kids[j].name
	})
	for i, c := range kids {
		branch, indent := "├── ", "│   "
		if i == len(kids)-1 {
			branch, indent = "└── ", "    "
		}
		text := c.name
		if c.label != "" {
			text = c.label
		}
		switch {
		case c.entry != nil:
			text = treeEntry(text, *c.entry)
		case c.isDir:
			text = blue(text)
		}
		lines = append(lines, prefix+branch+text)
		lines = appendChildren(lines, c, prefix+indent)
	}
	return lines
}

// treeEntry colors a file like git-status-tree: green when fully staged, red
// when anything is left unstaged. The annotation reads "(M+)" for staged,
// "(M)" for unstaged and "(M+M)" for both, with each part in its own color.
func treeEntry(name string, e Entry) string {
	switch {
	case e.Untracked():
		return red(name + " (?)")
	case e.Unmerged():
		return red(name + " (" + string([]byte{e.X, e.Y}) + ")")
	case !e.Unstaged():
		return green(name + " (" + string(e.X) + "+)")
	case !e.Staged():
		return red(name + " (" + string(e.Y) + ")")
	}
	return red(name+" (") + green(string(e.X)+"+") + red(string(e.Y)+")")
}
