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
	"sync"
	"syscall"
	"time"
	"unicode/utf8"

	"golang.org/x/term"
)

type view int

const (
	listView view = iota
	treeView
)

// keyCode is a key on the keyboard. What it does depends on the app's state.
type keyCode int

const (
	keyChar keyCode = iota // a printable character, in key.r
	keyQuit                // Ctrl-C, or the end of the input
	keyEnter
	keyEsc
	keyBackspace
	keyTab
	keyUp
	keyDown
	keyPageUp
	keyPageDown
	keyHome
	keyEnd
)

// key is one key press.
type key struct {
	code keyCode
	r    rune // the character, for keyChar
}

func char(r rune) key { return key{code: keyChar, r: r} }

// is reports whether k is the character r.
func (k key) is(r rune) bool { return k.code == keyChar && k.r == r }

type app struct {
	interval time.Duration
	dir      string  // the directory being watched
	status   *Status // nil until the first run in dir finishes
	view     view
	scroll   int
	picker   *picker // the worktree list, while it is shown
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

Live git status. Keys: q quit, t/Tab toggle list/tree, w worktrees, ↑↓/jk scroll,
r refresh.

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
	// The directory to watch changes when the user picks another worktree.
	var mu sync.Mutex
	dir := cfg.dir
	watched := func() string {
		mu.Lock()
		defer mu.Unlock()
		return dir
	}

	done := make(chan struct{})
	defer close(done)
	results := make(chan Status)
	kick := make(chan struct{}, 1)
	roots := map[string]string{} // worktree of each watched directory
	status := func() Status {
		d := watched()
		st := runStatus(d, cfg.untracked)
		if st.Err == nil {
			if roots[d] == "" {
				roots[d] = gitRoot(d)
			}
			st.Root = roots[d]
		}
		return st
	}
	go poll(status, cfg.interval, results, kick, done)
	keys := make(chan key)
	go readKeys(in, keys)

	// The worktree list is only polled while it is shown.
	worktrees := make(chan Worktrees)
	kickWorktrees := make(chan struct{}, 1)
	var stopWorktrees chan struct{}
	pollWorktrees := func(on bool) {
		if stopWorktrees != nil {
			close(stopWorktrees)
			stopWorktrees = nil
		}
		if on {
			stopWorktrees = make(chan struct{})
			load := func() Worktrees { return loadWorktrees(watched(), cfg.untracked) }
			go poll(load, cfg.interval, worktrees, kickWorktrees, stopWorktrees)
		}
	}
	defer pollWorktrees(false)

	a := &app{interval: cfg.interval, view: cfg.view, dir: cfg.dir}
	return a.loop(events{
		results:       results,
		worktrees:     worktrees,
		keys:          keys,
		quit:          quit,
		tick:          tick,
		size:          size,
		kick:          kick,
		kickWorktrees: kickWorktrees,
		watch: func(d string) {
			mu.Lock()
			dir = d
			mu.Unlock()
			select {
			case kick <- struct{}{}:
			default:
			}
		},
		pollWorktrees: pollWorktrees,
	}, out)
}

// events are the inputs of the main loop, and what it controls.
type events struct {
	results       <-chan Status
	worktrees     <-chan Worktrees // while the worktree list is shown
	keys          <-chan key
	quit          <-chan os.Signal
	tick          <-chan time.Time // when to check the terminal size
	size          func() (width, height int, err error)
	kick          chan<- struct{}  // asks for a git status now
	kickWorktrees chan<- struct{}  // asks for the worktree list now
	watch         func(dir string) // watches another directory from now on
	pollWorktrees func(on bool)    // starts or stops polling the worktree list
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
			if st.Dir != a.dir {
				continue // started before switching to another worktree
			}
			a.status = &st
		case wts := <-ev.worktrees:
			if a.picker == nil {
				continue // started before the list was closed
			}
			a.picker.wts = &wts
		case k := <-ev.keys:
			dir, picking := a.dir, a.picker != nil
			quit, refresh := a.handleKey(k)
			if quit {
				return nil
			}
			if refresh {
				for _, c := range []chan<- struct{}{ev.kick, ev.kickWorktrees} {
					select {
					case c <- struct{}{}:
					default: // a refresh is already pending
					}
				}
			}
			if a.dir != dir {
				ev.watch(a.dir)
			}
			if picking != (a.picker != nil) {
				ev.pollWorktrees(!picking)
			}
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

// poll calls fetch repeatedly, never overlapping calls, waiting interval
// between them. A send on kick skips the wait; closing done stops it.
func poll[T any](fetch func() T, interval time.Duration, results chan<- T, kick <-chan struct{}, done <-chan struct{}) {
	for {
		select {
		case results <- fetch():
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
			keys <- key{code: keyQuit}
			return
		}
	}
}

// decodeKeys decodes one read of raw terminal input into key presses.
func decodeKeys(b []byte) []key {
	var keys []key
	for i := 0; i < len(b); i++ {
		switch c := b[i]; {
		case c == 3: // Ctrl-C
			keys = append(keys, key{code: keyQuit})
		case c == '\r' || c == '\n':
			keys = append(keys, key{code: keyEnter})
		case c == '\t':
			keys = append(keys, key{code: keyTab})
		case c == 0x7f || c == 0x08:
			keys = append(keys, key{code: keyBackspace})
		case c == 0x1b:
			// Escape on its own, or followed by something other than a
			// cursor key sequence (e.g. Alt-x), is the Escape key.
			if i+1 == len(b) || (b[i+1] != '[' && b[i+1] != 'O') {
				keys = append(keys, key{code: keyEsc})
				continue
			}
			j := i + 2
			for j < len(b) && b[j] >= '0' && b[j] <= '9' {
				j++
			}
			if j >= len(b) { // incomplete sequence at the end of the read
				i = j
				continue
			}
			switch seq := string(b[i+2 : j+1]); seq {
			case "A":
				keys = append(keys, key{code: keyUp})
			case "B":
				keys = append(keys, key{code: keyDown})
			case "5~":
				keys = append(keys, key{code: keyPageUp})
			case "6~":
				keys = append(keys, key{code: keyPageDown})
			case "H", "1~", "7~":
				keys = append(keys, key{code: keyHome})
			case "F", "4~", "8~":
				keys = append(keys, key{code: keyEnd})
			}
			i = j
		case c < 0x20: // other control characters
		default:
			r, n := utf8.DecodeRune(b[i:])
			if r != utf8.RuneError {
				keys = append(keys, char(r))
			}
			i += n - 1
		}
	}
	return keys
}

func (a *app) bodyHeight() int { return max(a.height-2, 1) }

// handleKey applies a key press to the app. It reports whether the key asks
// to quit or to refresh now.
func (a *app) handleKey(k key) (quit, refresh bool) {
	if a.picker != nil {
		return a.handlePickerKey(k)
	}
	switch {
	case k.code == keyQuit, k.is('q'), k.is('Q'):
		return true, false
	case k.is('r'), k.is('R'):
		return false, true
	case k.is('w'), k.is('W'):
		a.openPicker()
	case k.code == keyTab, k.is('t'), k.is('T'):
		a.view = 1 - a.view
		a.scroll = 0
	case k.code == keyUp, k.is('k'):
		a.scroll--
	case k.code == keyDown, k.is('j'):
		a.scroll++
	case k.code == keyPageUp:
		a.scroll -= a.bodyHeight()
	case k.code == keyPageDown, k.is(' '):
		a.scroll += a.bodyHeight()
	case k.code == keyHome, k.is('g'):
		a.scroll = 0
	case k.code == keyEnd, k.is('G'):
		a.scroll = 1 << 30 // clamped when drawing
	}
	return false, false
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
	body, sel := a.body()
	scroll := &a.scroll
	footer := " q quit  t/Tab toggle view  w worktrees  ↑↓/jk scroll  r refresh"
	if a.picker != nil {
		scroll = &a.picker.scroll
		if sel >= 0 { // keep the selection on the screen
			*scroll = max(min(*scroll, sel), sel-bodyH+1)
		}
		footer = " enter switch  ↑↓/jk select  / search  w/esc back  r refresh  q quit"
		if a.picker.searching {
			footer = " type to search  enter switch  ↑↓ select  esc clear  ctrl-c quit"
		}
	}
	*scroll = min(max(*scroll, 0), max(len(body)-bodyH, 0))

	rows := make([]string, 0, a.height)
	rows = append(rows, ansiReverse+pad(truncate(a.header(), a.width), a.width)+ansiReset)
	for i := range bodyH {
		line := ""
		if n := *scroll + i; n < len(body) {
			line = body[n]
		}
		rows = append(rows, truncate(line, a.width))
	}
	if a.height > 1 {
		if len(body) > bodyH {
			pos := fmt.Sprintf("%d-%d/%d ", *scroll+1, min(*scroll+bodyH, len(body)), len(body))
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
	if a.picker != nil {
		return a.pickerHeader()
	}
	name := "LIST"
	if a.view == treeView {
		name = "TREE"
	}
	left := " " + name
	if a.status != nil && a.status.Err == nil {
		if a.status.Root != "" {
			left += "  │  " + Worktree{Path: a.status.Root}.Name()
		}
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

// body returns the lines below the header, and which of them is selected
// (-1 for none).
func (a *app) body() ([]string, int) {
	switch {
	case a.picker != nil:
		return a.pickerBody()
	case a.status == nil:
		return a.centered("Running git status…"), -1
	case a.status.Err != nil:
		return a.errorLines(a.status.Err), -1
	case a.view == treeView:
		return renderTree(a.status.Entries), -1
	default:
		return renderList(a.status.Entries), -1
	}
}

// centered returns lines that show msg in the middle of the body.
func (a *app) centered(msg string) []string {
	lines := make([]string, a.bodyHeight()/2)
	return append(lines, strings.Repeat(" ", max((a.width-visibleLen(msg))/2, 0))+msg)
}

// errorLines shows a git error, which is retried every interval.
func (a *app) errorLines(err error) []string {
	var lines []string
	for _, l := range strings.Split(err.Error(), "\n") {
		lines = append(lines, red(l))
	}
	return append(lines, "", dim(fmt.Sprintf("Retrying every %s…", a.interval)))
}
