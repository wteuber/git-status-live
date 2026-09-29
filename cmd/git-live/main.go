// git-live shows a live, auto-refreshing `git status` in the terminal.
// Installed on the PATH, git runs it as `git live`.
package main

import (
	"errors"
	"flag"
	"fmt"
	"io"
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
	interval time.Duration
	status   *Status // nil until the first run finishes
	view     view
	scroll   int
	width    int
	height   int
}

// config is what the command line asks for.
type config struct {
	dir       string
	interval  time.Duration
	untracked bool
	view      view
}

const usage = `Usage: git live [-i interval] [-u] [--view list|tree] [path]

Live git status. Keys: q quit, t/Tab toggle list/tree, ↑↓/jk scroll, r refresh.

  -i duration        refresh interval, e.g. 250ms or 2s (default 500ms)
  -u, --untracked    show untracked files in new directories
  --view list|tree   view to start in (default: git config live.view, or list)
  -h, --help         show this help

Source: https://github.com/wteuber/git-status-live
`

func main() {
	os.Exit(cli(os.Args[1:], os.Stdout, os.Stderr, runTerminal))
}

// cli parses args, calls start with the resulting config and returns the
// process exit code: 0 on success or -h/--help, 1 if start fails or the git
// config is invalid, 2 for bad usage.
func cli(args []string, stdout, stderr io.Writer, start func(config) error) int {
	fs := flag.NewFlagSet("git live", flag.ContinueOnError)
	fs.SetOutput(io.Discard) // errors and usage are printed below
	cfg := config{}
	fs.DurationVar(&cfg.interval, "i", 500*time.Millisecond, "")
	fs.BoolVar(&cfg.untracked, "u", false, "")
	fs.BoolVar(&cfg.untracked, "untracked", false, "")
	viewSet := false
	fs.Func("view", "", func(s string) (err error) {
		cfg.view, err = parseView(s)
		viewSet = true
		return err
	})
	err := fs.Parse(args)
	if errors.Is(err, flag.ErrHelp) {
		fmt.Fprint(stdout, usage)
		return 0
	}
	if err == nil {
		cfg.dir, err = parseArgs(fs.Args(), cfg.interval)
	}
	if err != nil {
		fmt.Fprintln(stderr, "git live:", err)
		fmt.Fprint(stderr, usage)
		return 2
	}
	// Without --view, start in the view from the git config, if any.
	if !viewSet {
		if s := gitConfig(cfg.dir, "live.view"); s != "" {
			if cfg.view, err = parseView(s); err != nil {
				fmt.Fprintln(stderr, "git live: git config live.view:", err)
				return 1
			}
		}
	}
	if err := start(cfg); err != nil {
		fmt.Fprintln(stderr, "git live:", err)
		return 1
	}
	return 0
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

// parseView parses the name of a view, "list" or "tree", in any case.
func parseView(s string) (view, error) {
	switch strings.ToLower(s) {
	case "list":
		return listView, nil
	case "tree":
		return treeView, nil
	}
	return listView, fmt.Errorf("unknown view %q, want list or tree", s)
}

// runTerminal sets up the terminal and shows the live view until the user
// quits. Everything after the setup happens in runApp.
func runTerminal(cfg config) error {
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

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, os.Interrupt, syscall.SIGTERM)
	// Windows has no SIGWINCH, so poll the terminal size instead.
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()
	size := func() (int, int, error) { return term.GetSize(outFd) }
	return runApp(cfg, os.Stdin, os.Stdout, size, quit, ticker.C)
}

// runApp connects git polling and key input to the main loop and runs it
// until the user quits. It reads keys from in and draws to out.
func runApp(cfg config, in io.Reader, out io.Writer, size func() (int, int, error), quit <-chan os.Signal, tick <-chan time.Time) error {
	done := make(chan struct{})
	defer close(done)
	results := make(chan Status)
	kick := make(chan struct{}, 1)
	status := func() Status { return runStatus(cfg.dir, cfg.untracked) }
	go poll(status, cfg.interval, results, kick, done)
	keys := make(chan key)
	go readKeys(in, keys)

	a := &app{interval: cfg.interval, view: cfg.view}
	return a.loop(events{
		results: results,
		keys:    keys,
		quit:    quit,
		tick:    tick,
		size:    size,
		kick:    kick,
	}, out)
}

// events are the inputs of the main loop.
type events struct {
	results <-chan Status
	keys    <-chan key
	quit    <-chan os.Signal
	tick    <-chan time.Time // when to check the terminal size
	size    func() (width, height int, err error)
	kick    chan<- struct{} // asks poll for a refresh now
}

// loop handles events until the user quits, writing a frame to out only
// when the screen content changed.
func (a *app) loop(ev events, out io.Writer) error {
	if w, h, err := ev.size(); err == nil {
		a.width, a.height = w, h
	}
	var last string
	for {
		select {
		case st := <-ev.results:
			a.status = &st
		case k := <-ev.keys:
			if k == keyQuit {
				return nil
			}
			if k == keyRefresh {
				select {
				case ev.kick <- struct{}{}:
				default: // a refresh is already pending
				}
			}
			a.handleKey(k)
		case <-ev.tick:
			if w, h, err := ev.size(); err == nil {
				a.width, a.height = w, h
			}
		case <-ev.quit:
			return nil
		}
		if frame := a.draw(); frame != last {
			if _, err := io.WriteString(out, frame); err != nil {
				return err
			}
			last = frame
		}
	}
}

// poll calls status repeatedly, never overlapping calls, waiting interval
// between them. A send on kick skips the wait; closing done stops it.
func poll(status func() Status, interval time.Duration, results chan<- Status, kick <-chan struct{}, done <-chan struct{}) {
	for {
		select {
		case results <- status():
		case <-done:
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

// readKeys decodes raw terminal input from r into keys. When r fails or
// ends, it sends keyQuit.
func readKeys(r io.Reader, keys chan<- key) {
	buf := make([]byte, 64)
	for {
		n, err := r.Read(buf)
		for _, k := range decodeKeys(buf[:n]) {
			keys <- k
		}
		if err != nil {
			keys <- keyQuit
			return
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
			// Shorten the key hints, not the position, on narrow terminals.
			w := max(a.width-len(pos), 0)
			if r := []rune(footer); len(r) >= w {
				footer = string(r[:max(w-1, 0)])
			}
			footer = pad(footer, w) + pos
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
