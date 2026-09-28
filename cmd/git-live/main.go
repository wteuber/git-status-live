// git-live shows a live, auto-refreshing `git status` in the terminal.
// Installed on the PATH, git runs it as `git live`.
package main

import (
	"flag"
	"fmt"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"golang.org/x/term"
)

type view int

const (
	listView view = iota
	treeView
)

type key int

const (
	keyQuit key = iota
	keyToggle
	keyRefresh
	keyUp
	keyDown
	keyPageUp
	keyPageDown
	keyTop
	keyBottom
)

type app struct {
	dir      string
	interval time.Duration
	status   *Status // nil until the first run finishes
	view     view
	scroll   int
	width    int
	height   int
}

func main() {
	interval := flag.Duration("i", 500*time.Millisecond, "")
	var untracked bool
	flag.BoolVar(&untracked, "u", false, "")
	flag.BoolVar(&untracked, "untracked", false, "")
	flag.Usage = func() {
		fmt.Fprint(flag.CommandLine.Output(), `Usage: git live [-i interval] [-u] [path]

Live git status. Keys: q quit, t/Tab toggle list/tree, ↑↓/jk scroll, r refresh.

  -i duration        refresh interval, e.g. 250ms or 2s (default 500ms)
  -u, --untracked    show untracked files in new directories
  -h                 show this help
`)
	}
	flag.Parse()
	dir, err := parseArgs(flag.Args(), *interval)
	if err != nil {
		fmt.Fprintln(os.Stderr, "git live:", err)
		flag.Usage()
		os.Exit(2)
	}
	if err := run(dir, *interval, untracked); err != nil {
		fmt.Fprintln(os.Stderr, "git live:", err)
		os.Exit(1)
	}
}

// parseArgs validates the command line and returns the directory to watch.
func parseArgs(args []string, interval time.Duration) (string, error) {
	if interval <= 0 {
		return "", fmt.Errorf("interval must be positive, got %s", interval)
	}
	switch len(args) {
	case 0:
		return ".", nil
	case 1:
		return args[0], nil
	}
	return "", fmt.Errorf("expected at most one path, got %q (flags go before the path)", args)
}

// run shows the live view until the user quits. With untracked set, new
// directories are listed file by file instead of as a single entry.
func run(dir string, interval time.Duration, untracked bool) error {
	inFd, outFd := int(os.Stdin.Fd()), int(os.Stdout.Fd())
	if !term.IsTerminal(inFd) || !term.IsTerminal(outFd) {
		return fmt.Errorf("stdin and stdout must be a terminal")
	}
	defer enableVT()()
	oldState, err := term.MakeRaw(inFd)
	if err != nil {
		return err
	}
	defer term.Restore(inFd, oldState)
	// Alternate screen, hidden cursor, and no auto-wrap, so a line that is
	// wider than measured (e.g. wide CJK characters) can't spill into the next row.
	os.Stdout.WriteString("\x1b[?1049h\x1b[?25l\x1b[?7l")
	defer os.Stdout.WriteString("\x1b[?7h\x1b[?25h\x1b[?1049l")

	a := &app{dir: dir, interval: interval}
	a.width, a.height, _ = term.GetSize(outFd)

	results := make(chan Status)
	kick := make(chan struct{}, 1)
	go poll(dir, untracked, interval, results, kick)
	keys := make(chan key)
	go readKeys(keys)
	sigs := make(chan os.Signal, 1)
	signal.Notify(sigs, os.Interrupt, syscall.SIGTERM)

	// Windows has no SIGWINCH, so poll the terminal size instead.
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()
	var last string
	for {
		select {
		case st := <-results:
			a.status = &st
		case k := <-keys:
			if k == keyQuit {
				return nil
			}
			if k == keyRefresh {
				select {
				case kick <- struct{}{}:
				default:
				}
			}
			a.handleKey(k)
		case <-ticker.C:
			if w, h, err := term.GetSize(outFd); err == nil {
				a.width, a.height = w, h
			}
		case <-sigs:
			return nil
		}
		if frame := a.draw(); frame != last {
			os.Stdout.WriteString(frame)
			last = frame
		}
	}
}

// poll runs git status repeatedly, never overlapping runs, waiting interval
// between them. A send on kick skips the wait.
func poll(dir string, untracked bool, interval time.Duration, results chan<- Status, kick <-chan struct{}) {
	for {
		results <- runStatus(dir, untracked)
		select {
		case <-time.After(interval):
		case <-kick:
		}
	}
}

// readKeys decodes raw terminal input into keys.
func readKeys(keys chan<- key) {
	buf := make([]byte, 64)
	for {
		n, err := os.Stdin.Read(buf)
		if err != nil {
			keys <- keyQuit
			return
		}
		for _, k := range decodeKeys(buf[:n]) {
			keys <- k
		}
	}
}

func decodeKeys(b []byte) []key {
	var keys []key
	for i := 0; i < len(b); i++ {
		switch b[i] {
		case 'q', 'Q', 3: // 3 is Ctrl-C
			keys = append(keys, keyQuit)
		case 't', 'T', '\t':
			keys = append(keys, keyToggle)
		case 'r', 'R':
			keys = append(keys, keyRefresh)
		case 'k':
			keys = append(keys, keyUp)
		case 'j':
			keys = append(keys, keyDown)
		case ' ':
			keys = append(keys, keyPageDown)
		case 'g':
			keys = append(keys, keyTop)
		case 'G':
			keys = append(keys, keyBottom)
		case 0x1b:
			if i+2 >= len(b) || (b[i+1] != '[' && b[i+1] != 'O') {
				continue // lone Escape, ignore
			}
			j := i + 2
			for j < len(b) && b[j] >= '0' && b[j] <= '9' {
				j++
			}
			if j >= len(b) {
				i = j
				continue
			}
			switch seq := string(b[i+2 : j+1]); seq {
			case "A":
				keys = append(keys, keyUp)
			case "B":
				keys = append(keys, keyDown)
			case "5~":
				keys = append(keys, keyPageUp)
			case "6~":
				keys = append(keys, keyPageDown)
			case "H", "1~", "7~":
				keys = append(keys, keyTop)
			case "F", "4~", "8~":
				keys = append(keys, keyBottom)
			}
			i = j
		}
	}
	return keys
}

func (a *app) bodyHeight() int { return max(a.height-2, 1) }

func (a *app) handleKey(k key) {
	switch k {
	case keyToggle:
		a.view = 1 - a.view
		a.scroll = 0
	case keyUp:
		a.scroll--
	case keyDown:
		a.scroll++
	case keyPageUp:
		a.scroll -= a.bodyHeight()
	case keyPageDown:
		a.scroll += a.bodyHeight()
	case keyTop:
		a.scroll = 0
	case keyBottom:
		a.scroll = 1 << 30 // clamped when drawing
	}
}

// draw returns the escape sequence that paints the whole screen.
func (a *app) draw() string {
	var b strings.Builder
	for i, line := range a.frame() {
		fmt.Fprintf(&b, "\x1b[%d;1H%s\x1b[K", i+1, line)
	}
	return b.String()
}

// frame returns the screen content, one string per terminal row.
func (a *app) frame() []string {
	if a.width <= 0 || a.height <= 0 {
		return nil
	}
	bodyH := a.bodyHeight()
	body := a.body()
	a.scroll = min(max(a.scroll, 0), max(len(body)-bodyH, 0))

	rows := make([]string, 0, a.height)
	rows = append(rows, ansiReverse+pad(truncate(a.header(), a.width), a.width)+ansiReset)
	for i := range bodyH {
		line := ""
		if n := a.scroll + i; n < len(body) {
			line = body[n]
		}
		rows = append(rows, truncate(line, a.width))
	}
	if a.height > 1 {
		footer := " q quit  t/Tab toggle view  ↑↓/jk scroll  r refresh"
		if len(body) > bodyH {
			pos := fmt.Sprintf("%d-%d/%d ", a.scroll+1, min(a.scroll+bodyH, len(body)), len(body))
			footer = pad(footer, a.width-len(pos)) + pos
		}
		rows = append(rows, dim(truncate(footer, a.width)))
	}
	return rows
}

func (a *app) header() string {
	name := "LIST"
	if a.view == treeView {
		name = "TREE"
	}
	left := " " + name
	if a.status != nil && a.status.Err == nil {
		left += "  │  " + a.status.Branch
		if s := counts(a.status.Entries); s != "" {
			left += "  │  " + s
		}
	}
	return left
}

func counts(entries []Entry) string {
	var staged, unstaged, untracked, conflicts int
	for _, e := range entries {
		switch {
		case e.Untracked():
			untracked++
		case e.Unmerged():
			conflicts++
		default:
			if e.Staged() {
				staged++
			}
			if e.Unstaged() {
				unstaged++
			}
		}
	}
	var parts []string
	for _, c := range []struct {
		n    int
		name string
	}{{staged, "staged"}, {unstaged, "unstaged"}, {untracked, "untracked"}, {conflicts, "conflicts"}} {
		if c.n > 0 {
			parts = append(parts, fmt.Sprintf("%d %s", c.n, c.name))
		}
	}
	return strings.Join(parts, ", ")
}

func (a *app) body() []string {
	switch {
	case a.status == nil:
		msg := "Running git status…"
		lines := make([]string, a.bodyHeight()/2)
		return append(lines, strings.Repeat(" ", max((a.width-visibleLen(msg))/2, 0))+msg)
	case a.status.Err != nil:
		var lines []string
		for _, l := range strings.Split(a.status.Err.Error(), "\n") {
			lines = append(lines, red(l))
		}
		return append(lines, "", dim(fmt.Sprintf("Retrying every %s…", a.interval)))
	case a.view == treeView:
		return renderTree(a.status.Entries)
	default:
		return renderList(a.status.Entries)
	}
}
