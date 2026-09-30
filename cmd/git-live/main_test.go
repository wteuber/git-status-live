package main

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// press returns the key press for a key that isn't a character.
func press(c keyCode) key { return key{code: c} }

func TestDecodeKeys(t *testing.T) {
	got := decodeKeys([]byte("t\tjk\x1b[A\x1b[B\x1b[5~\x1b[6~\r\x7fé\x03"))
	want := []key{char('t'), press(keyTab), char('j'), char('k'), press(keyUp), press(keyDown),
		press(keyPageUp), press(keyPageDown), press(keyEnter), press(keyBackspace), char('é'), press(keyQuit)}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("decodeKeys = %v, want %v", got, want)
	}
}

func TestHandleKeyActions(t *testing.T) {
	for _, tc := range []struct {
		key           key
		quit, refresh bool
		view          view
	}{
		{char('q'), true, false, listView},
		{char('Q'), true, false, listView},
		{press(keyQuit), true, false, listView},
		{char('r'), false, true, listView},
		{char('R'), false, true, listView},
		{char('t'), false, false, treeView},
		{char('T'), false, false, treeView},
		{press(keyTab), false, false, treeView},
		{char('x'), false, false, listView},
		{press(keyEnter), false, false, listView},
	} {
		a := &app{width: 40, height: 10}
		quit, refresh := a.handleKey(tc.key)
		if quit != tc.quit || refresh != tc.refresh || a.view != tc.view {
			t.Errorf("key %+v: quit=%v refresh=%v view=%v, want %v %v %v",
				tc.key, quit, refresh, a.view, tc.quit, tc.refresh, tc.view)
		}
	}
}

func TestParseArgs(t *testing.T) {
	for _, tc := range []struct {
		args     []string
		interval time.Duration
		dir      string
		wantErr  bool
	}{
		{nil, time.Second, ".", false},
		{[]string{"repo"}, time.Second, "repo", false},
		{[]string{"repo", "-i", "1s"}, time.Second, "", true},
		{nil, 0, "", true},
		{nil, -time.Second, "", true},
	} {
		dir, err := parseArgs(tc.args, tc.interval)
		if (err != nil) != tc.wantErr || dir != tc.dir {
			t.Errorf("parseArgs(%q, %s) = %q, %v", tc.args, tc.interval, dir, err)
		}
	}
}

func TestFrame(t *testing.T) {
	a := &app{width: 90, height: 5}

	// Before the first result: a static placeholder in the body.
	if got := plain(a.frame()); !strings.Contains(got, "Running git status…") {
		t.Errorf("first frame missing placeholder:\n%s", got)
	}

	st := Status{Branch: "main", Entries: sample}
	a.status = &st
	rows := a.frame()
	if len(rows) != 5 {
		t.Fatalf("got %d rows, want 5", len(rows))
	}
	for i, r := range rows {
		if visibleLen(r) > a.width {
			t.Errorf("row %d too wide: %q", i, stripANSI(r))
		}
	}
	header := stripANSI(rows[0])
	for _, s := range []string{"LIST", "main", "4 staged, 2 unstaged, 1 untracked"} {
		if !strings.Contains(header, s) {
			t.Errorf("header %q missing %q", header, s)
		}
	}
	if footer := stripANSI(rows[4]); !strings.Contains(footer, "1-3/") {
		t.Errorf("footer %q missing scroll position", footer)
	}

	// Identical status gives an identical frame, so nothing is redrawn.
	again := Status{Branch: "main", Entries: sample}
	before := a.draw()
	a.status = &again
	if a.draw() != before {
		t.Error("unchanged status produced a different frame")
	}

	a.handleKey(char('t'))
	a.handleKey(char('G'))
	rows = a.frame()
	if !strings.Contains(stripANSI(rows[0]), "TREE") || !strings.Contains(stripANSI(rows[3]), "both.go (M+M)") {
		t.Errorf("tree view bottom:\n%s", plain(rows))
	}
}

func TestHeaderShowsWorktree(t *testing.T) {
	a := &app{width: 80, height: 5, status: &Status{Branch: "feat", Root: "/src/repo-agent", Entries: sample[:1]}}
	if got := strings.TrimSpace(stripANSI(a.frame()[0])); got != "LIST  │  repo-agent  │  feat  │  1 staged" {
		t.Errorf("header = %q", got)
	}
	// On error, the header shows neither the worktree nor the branch.
	a.status = &Status{Err: errors.New("fatal"), Root: "/src/repo-agent"}
	if got := strings.TrimSpace(stripANSI(a.frame()[0])); got != "LIST" {
		t.Errorf("error header = %q", got)
	}
}

func TestCLI(t *testing.T) {
	isolateGitConfig(t) // live.view in the user's config must not change the view
	for _, tc := range []struct {
		name     string
		args     []string
		startErr error
		code     int
		cfg      *config // nil if start must not be called
		stdout   string
		stderr   string
	}{
		{name: "defaults", args: nil, code: 0,
			cfg: &config{dir: ".", interval: 500 * time.Millisecond}},
		{name: "all options", args: []string{"-i", "250ms", "-u", "repo"}, code: 0,
			cfg: &config{dir: "repo", interval: 250 * time.Millisecond, untracked: true}},
		{name: "long untracked", args: []string{"--untracked"}, code: 0,
			cfg: &config{dir: ".", interval: 500 * time.Millisecond, untracked: true}},
		{name: "view", args: []string{"--view", "tree"}, code: 0,
			cfg: &config{dir: ".", interval: 500 * time.Millisecond, view: treeView}},
		{name: "view in any case", args: []string{"-view=TREE"}, code: 0,
			cfg: &config{dir: ".", interval: 500 * time.Millisecond, view: treeView}},
		{name: "unknown view", args: []string{"--view", "grid"}, code: 2, stderr: `unknown view "grid", want list or tree`},
		{name: "help", args: []string{"-h"}, code: 0, stdout: "Usage: git live"},
		{name: "long help", args: []string{"--help"}, code: 0, stdout: "Usage: git live"},
		{name: "help links to the source", args: []string{"--help"}, code: 0,
			stdout: "Source: https://github.com/wteuber/git-status-live\n"},
		{name: "help after other flags", args: []string{"-u", "--help"}, code: 0, stdout: "Usage: git live"},
		{name: "unknown flag", args: []string{"-x"}, code: 2, stderr: "git live: flag provided but not defined: -x"},
		{name: "bad interval", args: []string{"-i", "0s"}, code: 2, stderr: "interval must be positive"},
		{name: "flag after path", args: []string{"repo", "-u"}, code: 2, stderr: "flags go before the path"},
		{name: "start fails", args: nil, startErr: errors.New("boom"), code: 1,
			cfg: &config{dir: ".", interval: 500 * time.Millisecond}, stderr: "git live: boom"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var stdout, stderr strings.Builder
			var got *config
			code := cli(tc.args, &stdout, &stderr, func(c config) error {
				got = &c
				return tc.startErr
			})
			if code != tc.code {
				t.Errorf("exit code = %d, want %d", code, tc.code)
			}
			if !reflect.DeepEqual(got, tc.cfg) {
				t.Errorf("config = %+v, want %+v", got, tc.cfg)
			}
			if !strings.Contains(stdout.String(), tc.stdout) || (tc.stdout == "" && stdout.Len() > 0) {
				t.Errorf("stdout = %q, want %q", stdout.String(), tc.stdout)
			}
			if !strings.Contains(stderr.String(), tc.stderr) || (tc.stderr == "" && stderr.Len() > 0) {
				t.Errorf("stderr = %q, want %q", stderr.String(), tc.stderr)
			}
			if tc.code == 2 && !strings.Contains(stderr.String(), "Usage: git live") {
				t.Errorf("usage error without usage text: %q", stderr.String())
			}
		})
	}
}

func TestCLIViewFromGitConfig(t *testing.T) {
	dir, git, _ := testRepo(t)
	run := func(args ...string) (int, *config, string) {
		var stderr strings.Builder
		var got *config
		code := cli(args, io.Discard, &stderr, func(c config) error { got = &c; return nil })
		return code, got, stderr.String()
	}

	git("config", "live.view", "tree")
	if code, cfg, _ := run(dir); code != 0 || cfg.view != treeView {
		t.Errorf("live.view=tree: code %d, config %+v; want tree", code, cfg)
	}
	// --view wins over the git config.
	if code, cfg, _ := run("--view", "list", dir); code != 0 || cfg.view != listView {
		t.Errorf("--view list: code %d, config %+v; want list", code, cfg)
	}

	git("config", "live.view", "grid")
	code, cfg, stderr := run(dir)
	if code != 1 || cfg != nil {
		t.Errorf("invalid live.view: code %d, config %+v; want 1 without starting", code, cfg)
	}
	if want := "git live: git config live.view: unknown view \"grid\", want list or tree\n"; stderr != want {
		t.Errorf("stderr = %q, want %q", stderr, want)
	}
	// An invalid git config doesn't matter when --view is given.
	if code, cfg, _ := run("--view", "tree", dir); code != 0 || cfg.view != treeView {
		t.Errorf("--view with invalid live.view: code %d, config %+v", code, cfg)
	}
}

// countingWriter records every frame the loop writes.
type countingWriter struct{ frames []string }

func (w *countingWriter) Write(p []byte) (int, error) {
	w.frames = append(w.frames, string(p))
	return len(p), nil
}

// loopHarness runs app.loop with channels the test controls. Sends on the
// unbuffered channels only complete once the loop has finished drawing the
// previous event, so the test can inspect the output without races after stop.
type loopHarness struct {
	results       chan Status
	worktrees     chan Worktrees
	keys          chan key
	quit          chan os.Signal
	tick          chan time.Time
	kick          chan struct{}
	kickWorktrees chan struct{}
	width         int
	out           countingWriter
	app           *app
	err           chan error
	watched       []string // directories the loop switched to, with " -u" if untracked
	polling       []bool   // calls to start (true) or stop (false) the worktree poll
}

func startLoop(t *testing.T) *loopHarness {
	t.Helper()
	h := &loopHarness{
		results:       make(chan Status),
		worktrees:     make(chan Worktrees),
		keys:          make(chan key),
		quit:          make(chan os.Signal),
		tick:          make(chan time.Time),
		kick:          make(chan struct{}, 1),
		kickWorktrees: make(chan struct{}, 1),
		width:         60,
		app:           &app{interval: time.Second},
		err:           make(chan error, 1),
	}
	ev := events{
		results: h.results, worktrees: h.worktrees, keys: h.keys, quit: h.quit, tick: h.tick,
		kick: h.kick, kickWorktrees: h.kickWorktrees,
		size: func() (int, int, error) { return h.width, 8, nil },
		// Called by the loop goroutine; the test reads them after stop.
		watch: func(dir string, untracked bool) {
			if untracked {
				dir += " -u"
			}
			h.watched = append(h.watched, dir)
		},
		pollWorktrees: func(on bool) { h.polling = append(h.polling, on) },
	}
	go func() { h.err <- h.app.loop(ev, &h.out) }()
	return h
}

// stop quits the loop with q and waits for it to return.
func (h *loopHarness) stop(t *testing.T) {
	t.Helper()
	h.keys <- char('q')
	if err := <-h.err; err != nil {
		t.Fatalf("loop returned %v", err)
	}
}

func TestLoopRedrawsOnlyOnChange(t *testing.T) {
	h := startLoop(t)
	h.results <- Status{Branch: "main", Entries: sample}
	h.results <- Status{Branch: "main", Entries: sample} // unchanged: no redraw
	h.tick <- time.Now()                                 // same size: no redraw
	h.results <- Status{Branch: "main"}                  // repo is now clean
	h.stop(t)

	if len(h.out.frames) != 2 {
		t.Fatalf("got %d frames, want 2", len(h.out.frames))
	}
	if !strings.Contains(stripANSI(h.out.frames[0]), "4 staged") {
		t.Errorf("first frame lacks status:\n%s", stripANSI(h.out.frames[0]))
	}
	if !strings.Contains(stripANSI(h.out.frames[1]), "nothing to commit") {
		t.Errorf("second frame not clean:\n%s", stripANSI(h.out.frames[1]))
	}
}

func TestLoopResize(t *testing.T) {
	h := startLoop(t)
	h.results <- Status{Branch: "main"}
	h.width = 40 // read by the loop only on the next tick
	h.tick <- time.Now()
	h.stop(t)
	if h.app.width != 40 || len(h.out.frames) != 2 {
		t.Errorf("width = %d, frames = %d; want 40, 2", h.app.width, len(h.out.frames))
	}
}

func TestLoopKeys(t *testing.T) {
	h := startLoop(t)
	h.results <- Status{Branch: "main", Entries: sample}
	h.keys <- char('t')
	h.keys <- char('r')
	h.keys <- char('R') // a refresh is already pending: must not block
	h.stop(t)

	if h.app.view != treeView || !strings.Contains(stripANSI(h.out.frames[len(h.out.frames)-1]), "TREE") {
		t.Error("toggle did not switch to the tree view")
	}
	select {
	case <-h.kick:
	default:
		t.Error("r did not request a refresh")
	}
}

func TestLoopWorktrees(t *testing.T) {
	h := startLoop(t)
	h.results <- Status{Branch: "main", Root: "/src/repo"}
	h.worktrees <- worktreeFixture() // the list isn't shown: ignored
	h.keys <- char('w')
	h.worktrees <- worktreeFixture()
	h.keys <- char('r') // refreshes both the status and the worktree list
	h.keys <- char('j')
	h.keys <- press(keyEnter)                                               // switch to repo-agent
	h.results <- Status{Branch: "main", Root: "/src/repo"}                  // stale: from before the switch
	h.results <- Status{Dir: "/src/repo-agent", Branch: "claude/fix-login"} // the new worktree
	h.stop(t)

	if !reflect.DeepEqual(h.polling, []bool{true, false}) {
		t.Errorf("worktree poll calls = %v, want start, then stop", h.polling)
	}
	if !reflect.DeepEqual(h.watched, []string{"/src/repo-agent"}) {
		t.Errorf("watched = %v", h.watched)
	}
	if len(h.kick) != 1 || len(h.kickWorktrees) != 1 {
		t.Errorf("r kicked status %d and worktrees %d times, want 1 each", len(h.kick), len(h.kickWorktrees))
	}
	frames := h.out.frames
	var sawList bool
	for _, f := range frames {
		sawList = sawList || strings.Contains(stripANSI(f), "5 worktrees")
	}
	if !sawList {
		t.Error("the worktree list was never drawn")
	}
	if last := stripANSI(frames[len(frames)-1]); !strings.Contains(last, "claude/fix-login") {
		t.Errorf("last frame doesn't show the new worktree:\n%s", last)
	}
	for _, f := range frames[len(frames)-2:] {
		if strings.Contains(stripANSI(f), "│  main") {
			t.Errorf("a stale status from the old worktree was drawn:\n%s", stripANSI(f))
		}
	}
}

func TestToggleUntracked(t *testing.T) {
	a := &app{width: 60, height: 5, status: &Status{Branch: "main"}}
	header := func() string { return strings.TrimSpace(stripANSI(a.frame()[0])) }
	a.handleKey(char('u'))
	if !a.untracked || !strings.HasPrefix(header(), "LIST -u  │") {
		t.Errorf("after u: untracked %v, header %q", a.untracked, header())
	}
	a.handleKey(char('t'))
	if !strings.HasPrefix(header(), "TREE -u  │") {
		t.Errorf("tree header = %q", header())
	}
	a.handleKey(char('U'))
	if a.untracked || !strings.HasPrefix(header(), "TREE  │") {
		t.Errorf("after U: untracked %v, header %q", a.untracked, header())
	}

	// In the worktree list too, but not while searching.
	a.handleKey(char('w'))
	a.handleKey(char('u'))
	if !a.untracked || header() != "WORKTREES -u" {
		t.Errorf("worktree list: untracked %v, header %q", a.untracked, header())
	}
	a.handleKey(char('/'))
	a.handleKey(char('u'))
	if !a.untracked || a.picker.query != "u" {
		t.Errorf("searching: untracked %v, query %q", a.untracked, a.picker.query)
	}
}

func TestLoopToggleUntracked(t *testing.T) {
	h := startLoop(t)
	h.app.dir = "/src/repo"
	h.results <- Status{Dir: "/src/repo", Branch: "before"}
	h.keys <- char('u')
	h.results <- Status{Dir: "/src/repo", Branch: "stale"} // started before the toggle
	h.results <- Status{Dir: "/src/repo", Untracked: true, Branch: "after"}
	h.keys <- char('u')
	h.stop(t)

	if want := []string{"/src/repo -u", "/src/repo"}; !reflect.DeepEqual(h.watched, want) {
		t.Errorf("watched = %q, want %q", h.watched, want)
	}
	for _, f := range h.out.frames {
		if strings.Contains(stripANSI(f), "stale") {
			t.Errorf("drew a status from before the toggle:\n%s", stripANSI(f))
		}
	}
	if last := stripANSI(h.out.frames[len(h.out.frames)-1]); !strings.Contains(last, "after") || strings.Contains(last, "-u") {
		t.Errorf("last frame:\n%s", last)
	}
}

func TestLoopQuitsOnSignal(t *testing.T) {
	h := startLoop(t)
	h.quit <- os.Interrupt
	if err := <-h.err; err != nil {
		t.Fatalf("loop returned %v", err)
	}
}

func TestLoopWriteError(t *testing.T) {
	a := &app{}
	results := make(chan Status, 1)
	results <- Status{}
	err := a.loop(events{
		results: results,
		size:    func() (int, int, error) { return 20, 5, nil },
	}, failingWriter{})
	if err == nil || err.Error() != "disk full" {
		t.Errorf("loop error = %v, want disk full", err)
	}
}

type failingWriter struct{}

func (failingWriter) Write([]byte) (int, error) { return 0, errors.New("disk full") }

func TestPoll(t *testing.T) {
	var calls, running, overlaps atomic.Int32
	status := func() Status {
		if running.Add(1) > 1 {
			overlaps.Add(1)
		}
		defer running.Add(-1)
		time.Sleep(time.Millisecond)
		return Status{Branch: fmt.Sprint(calls.Add(1))}
	}
	results := make(chan Status)
	kick := make(chan struct{}, 1)
	done := make(chan struct{})
	finished := make(chan struct{})
	go func() { poll(status, time.Hour, results, kick, done); close(finished) }()

	if st := <-results; st.Branch != "1" {
		t.Fatalf("first result = %q", st.Branch)
	}
	// The interval is an hour, so a second result can only come from the kick.
	kick <- struct{}{}
	select {
	case st := <-results:
		if st.Branch != "2" {
			t.Errorf("second result = %q", st.Branch)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("kick did not trigger a refresh")
	}
	close(done)
	select {
	case <-finished:
	case <-time.After(5 * time.Second):
		t.Fatal("poll did not stop when done was closed")
	}
	if overlaps.Load() > 0 {
		t.Error("status calls overlapped")
	}
}

func TestPollInterval(t *testing.T) {
	results := make(chan Status)
	done := make(chan struct{})
	defer close(done)
	go poll(func() Status { return Status{} }, 10*time.Millisecond, results, nil, done)
	start := time.Now()
	for range 3 {
		<-results
	}
	if d := time.Since(start); d < 20*time.Millisecond {
		t.Errorf("3 results after %s, want at least 2 intervals (20ms)", d)
	}
}

func TestReadKeys(t *testing.T) {
	keys := make(chan key, 10)
	readKeys(strings.NewReader("tj"), keys)
	close(keys)
	var got []key
	for k := range keys {
		got = append(got, k)
	}
	// End of input quits, so a closed terminal can't leave git-live hanging.
	want := []key{char('t'), char('j'), press(keyQuit)}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("keys = %v, want %v", got, want)
	}
}

func TestDecodeKeysMore(t *testing.T) {
	for _, tc := range []struct {
		in   string
		want []key
	}{
		{"\x1bOA\x1bOB", []key{press(keyUp), press(keyDown)}}, // application cursor mode
		{"\x1b[C\x1b[D\x1bOC\x1bOD", []key{press(keyRight), press(keyLeft), press(keyRight), press(keyLeft)}},
		{"\x1b[H\x1b[F\x1b[1~\x1b[4~\x1b[7~\x1b[8~", []key{press(keyHome), press(keyEnd),
			press(keyHome), press(keyEnd), press(keyHome), press(keyEnd)}},
		{"\x1b[", nil},                             // incomplete sequence at the end of a read
		{"\x1b[12", nil},                           // digits without a final byte
		{"\x1b[Zt", []key{char('t')}},              // unknown sequence (Shift-Tab) is skipped
		{"x\x1b", []key{char('x'), press(keyEsc)}}, // Escape on its own
		{"\x1bx", []key{press(keyEsc), char('x')}}, // Alt-x: Escape, then x
		{"\n\x08", []key{press(keyEnter), press(keyBackspace)}},
		{"\x01\x1a", nil}, // other control characters are ignored
		{"日本", []key{char('日'), char('本')}},
		{"\xe6\x97", nil},           // a character cut off at the end of a read
		{"\xffa", []key{char('a')}}, // invalid UTF-8 is skipped
	} {
		if got := decodeKeys([]byte(tc.in)); !reflect.DeepEqual(got, tc.want) {
			t.Errorf("decodeKeys(%q) = %v, want %v", tc.in, got, tc.want)
		}
	}
}

func TestScroll(t *testing.T) {
	var many []Entry
	for i := range 20 {
		many = append(many, Entry{X: '?', Y: '?', Path: fmt.Sprintf("f%02d", i)})
	}
	a := &app{width: 40, height: 7, status: &Status{Branch: "main", Entries: many}}
	// 5 body rows; the list has a title plus 20 files = 21 lines.
	firstRow := func() string { a.frame(); return strings.TrimSpace(stripANSI(a.frame()[1])) }

	for _, step := range []struct {
		key  key
		want string
	}{
		{press(keyDown), "f00"},
		{char('j'), "f01"},
		{char('k'), "f00"},
		{press(keyUp), "Untracked files:"},
		{press(keyUp), "Untracked files:"}, // can't scroll above the top
		{press(keyPageDown), "f04"},
		{char(' '), "f09"},
		{press(keyPageUp), "f04"},
		{press(keyEnd), "f15"},  // last page shows f15..f19
		{press(keyDown), "f15"}, // can't scroll past the end
		{press(keyHome), "Untracked files:"},
		{char('G'), "f15"},
		{char('g'), "Untracked files:"},
		{press(keyEsc), "Untracked files:"}, // does nothing outside the worktree list
	} {
		a.handleKey(step.key)
		if got := firstRow(); got != step.want {
			t.Fatalf("after key %+v first row = %q, want %q", step.key, got, step.want)
		}
	}
	if footer := stripANSI(a.frame()[6]); !strings.Contains(footer, "1-5/21") {
		t.Errorf("footer = %q", footer)
	}
}

func TestScrollSideways(t *testing.T) {
	long := Entry{X: '?', Y: '?', Path: "a/very/long/path/that/does/not/fit.txt"}
	// 20 columns: the widest line is 8 spaces + the 38 character path = 46.
	a := &app{width: 20, height: 5, status: &Status{Branch: "main", Entries: []Entry{long}}}
	row := func() string { return stripANSI(a.frame()[2]) }
	footer := func() string { return stripANSI(a.frame()[4]) }

	if got := row(); got != "        a/very/long/" {
		t.Fatalf("row = %q", got)
	}
	for _, step := range []struct {
		key       key
		row, col  string
		wantScrol int
	}{
		{char('l'), "very/long/path/that/", "col 11", 10}, // half the screen
		{press(keyRight), "path/that/does/not/f", "col 21", 20},
		{press(keyRight), "that/does/not/fit.txt"[1:], "col 27", 26}, // up to the end of the widest line
		{char('h'), "ong/path/that/does/n", "col 17", 16},
		{press(keyLeft), "  a/very/long/path/t", "col 7", 6},
		{press(keyLeft), "        a/very/long/", "", 0}, // not before the start
	} {
		a.handleKey(step.key)
		if got := row(); got != step.row || a.hscroll != step.wantScrol {
			t.Errorf("after %+v: row %q, scrolled %d; want %q, %d", step.key, got, a.hscroll, step.row, step.wantScrol)
		}
		if f := footer(); (step.col == "") != !strings.Contains(f, "col ") || !strings.Contains(f, step.col) {
			t.Errorf("after %+v: footer %q, want %q", step.key, f, step.col)
		}
	}

	// The header stays put, and toggling the view starts at the left.
	a.handleKey(char('l'))
	if got := stripANSI(a.frame()[0]); !strings.HasPrefix(got, " LIST") {
		t.Errorf("header scrolled: %q", got)
	}
	a.handleKey(char('t'))
	if a.hscroll != 0 {
		t.Errorf("toggle kept the sideways scroll: %d", a.hscroll)
	}

	// Content that fits can't be scrolled sideways.
	a = &app{width: 80, height: 5, status: &Status{Branch: "main", Entries: []Entry{long}}}
	a.handleKey(char('l'))
	a.frame()
	if a.hscroll != 0 {
		t.Errorf("scrolled content that fits: %d", a.hscroll)
	}
}

func TestFooterWithBothPositions(t *testing.T) {
	var many []Entry
	for i := range 20 {
		many = append(many, Entry{X: '?', Y: '?', Path: fmt.Sprintf("dir/%02d/%s", i, strings.Repeat("x", 60))})
	}
	a := &app{width: 40, height: 7, status: &Status{Branch: "main", Entries: many}}
	a.handleKey(char('l'))
	if footer := stripANSI(a.frame()[6]); !strings.HasSuffix(footer, "col 21  1-5/21 ") || visibleLen(footer) != 40 {
		t.Errorf("footer = %q", footer)
	}
}

func TestErrorScreen(t *testing.T) {
	a := &app{width: 80, height: 6, interval: 500 * time.Millisecond,
		status: &Status{Err: errors.New("fatal: not a git repository (or any of the parent directories): .git")}}
	rows := a.frame()
	if h := stripANSI(rows[0]); strings.Contains(h, "│") {
		t.Errorf("header shows repo details on error: %q", h)
	}
	if got := stripANSI(rows[1]); !strings.HasPrefix(got, "fatal: not a git repository") {
		t.Errorf("error row = %q", got)
	}
	if !strings.HasPrefix(rows[1], ansiRed) {
		t.Error("error is not red")
	}
	if got := stripANSI(rows[3]); got != "Retrying every 500ms…" {
		t.Errorf("retry row = %q", got)
	}
}

func TestTinyTerminal(t *testing.T) {
	a := &app{status: &Status{Branch: "main"}}
	if rows := a.frame(); rows != nil {
		t.Errorf("zero-size terminal drew %d rows", len(rows))
	}
	a.width, a.height = 10, 1
	if rows := a.frame(); len(rows) != 2 { // header and one body row, no footer
		t.Errorf("1-row terminal drew %d rows", len(rows))
	}
}

func TestNarrowFooterKeepsPosition(t *testing.T) {
	a := &app{width: 30, height: 4, status: &Status{Branch: "main", Entries: sample}}
	footer := a.frame()[3]
	if got := stripANSI(footer); !strings.HasSuffix(got, "toggle  1-2/12 ") || visibleLen(got) != 30 {
		t.Errorf("footer = %q", got)
	}
	if strings.Count(footer, ansiReset) != 1 {
		t.Errorf("footer styling interrupted: %q", footer)
	}
}

func TestPollStopsWhileDelivering(t *testing.T) {
	done := make(chan struct{})
	finished := make(chan struct{})
	// Nobody reads results, so poll blocks delivering the first one.
	go func() {
		poll(func() Status { return Status{} }, time.Hour, make(chan Status), nil, done)
		close(finished)
	}()
	close(done)
	select {
	case <-finished:
	case <-time.After(5 * time.Second):
		t.Fatal("poll did not stop while waiting to deliver a result")
	}
}

// screen collects the frames runApp draws and lets a test wait for one.
type screen struct {
	mu     sync.Mutex
	frames []string
	added  chan struct{}
}

func (s *screen) Write(p []byte) (int, error) {
	s.mu.Lock()
	s.frames = append(s.frames, stripANSI(string(p)))
	s.mu.Unlock()
	select {
	case s.added <- struct{}{}:
	default:
	}
	return len(p), nil
}

// waitFor waits until the latest frame contains all of want.
func (s *screen) waitFor(t *testing.T, want ...string) {
	t.Helper()
	deadline := time.After(10 * time.Second)
	for {
		s.mu.Lock()
		var last string
		if len(s.frames) > 0 {
			last = s.frames[len(s.frames)-1]
		}
		s.mu.Unlock()
		ok := true
		for _, w := range want {
			ok = ok && strings.Contains(last, w)
		}
		if ok {
			return
		}
		select {
		case <-s.added:
		case <-deadline:
			t.Fatalf("screen never showed %q; last frame:\n%s", want, last)
		}
	}
}

// TestRunApp runs the whole app, minus the terminal setup, against a real
// repository: real git polling, real key input and the real main loop.
func TestRunApp(t *testing.T) {
	dir, git, write := testRepo(t)
	in, keys := io.Pipe()
	defer keys.Close()
	scr := &screen{added: make(chan struct{}, 1)}
	size := func() (int, int, error) { return 80, 12, nil }
	cfg := config{dir: dir, interval: 20 * time.Millisecond}
	errc := make(chan error, 1)
	go func() { errc <- runApp(cfg, in, scr, size, nil, nil) }()

	scr.waitFor(t, "LIST", Worktree{Path: gitRoot(dir)}.Name(), "main", "nothing to commit")

	// Changes made while it runs show up without any key press.
	write("changed.txt", "two\n")
	scr.waitFor(t, "1 unstaged", "modified:   changed.txt")
	git("add", "changed.txt")
	scr.waitFor(t, "1 staged", "Changes to be committed:")

	io.WriteString(keys, "t")
	scr.waitFor(t, "TREE", "changed.txt (M+)")

	io.WriteString(keys, "q")
	select {
	case err := <-errc:
		if err != nil {
			t.Fatalf("runApp returned %v", err)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("q did not quit")
	}
}

// TestRunAppWorktrees browses, selects and watches a real linked worktree.
func TestRunAppWorktrees(t *testing.T) {
	dir, git, write := testRepo(t)
	feat := filepath.Join(t.TempDir(), "feat-wt")
	git("worktree", "add", "-q", "-b", "feat", feat)
	write("main-only.txt", "") // an untracked file in the main worktree
	if err := os.WriteFile(filepath.Join(feat, "feat-only.txt"), nil, 0o644); err != nil {
		t.Fatal(err)
	}

	in, keys := io.Pipe()
	defer keys.Close()
	scr := &screen{added: make(chan struct{}, 1)}
	size := func() (int, int, error) { return 120, 12, nil }
	cfg := config{dir: dir, interval: 20 * time.Millisecond}
	errc := make(chan error, 1)
	go func() { errc <- runApp(cfg, in, scr, size, nil, nil) }()

	mainName := Worktree{Path: gitRoot(dir)}.Name()
	scr.waitFor(t, "LIST", mainName, "main-only.txt")

	io.WriteString(keys, "w")
	scr.waitFor(t, "WORKTREES", "2 worktrees", "* "+mainName, "feat-wt", "feat", "1 untracked")

	// A change in the other worktree shows up in the list while it is open.
	if err := os.WriteFile(filepath.Join(feat, "second.txt"), nil, 0o644); err != nil {
		t.Fatal(err)
	}
	scr.waitFor(t, "WORKTREES", "2 untracked")

	io.WriteString(keys, "j\r") // select feat-wt and switch to it
	scr.waitFor(t, "LIST", "feat-wt", "feat", "feat-only.txt", "second.txt")

	// Back in the list, the watched worktree is now feat-wt.
	io.WriteString(keys, "w")
	scr.waitFor(t, "WORKTREES", "* feat-wt")
	io.WriteString(keys, "\x1b")
	scr.waitFor(t, "LIST", "feat-wt")

	// Search for the main worktree by its branch and switch back to it.
	io.WriteString(keys, "w")
	scr.waitFor(t, "WORKTREES", "2 worktrees")
	io.WriteString(keys, "/main")
	scr.waitFor(t, "1 of 2 worktrees", "/main▏", mainName)
	io.WriteString(keys, "\r")
	scr.waitFor(t, "LIST", mainName, "main-only.txt")

	io.WriteString(keys, "q")
	select {
	case err := <-errc:
		if err != nil {
			t.Fatalf("runApp returned %v", err)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("q did not quit")
	}
}

// TestRunAppToggleUntracked toggles -u at runtime against a real repository.
func TestRunAppToggleUntracked(t *testing.T) {
	dir, _, write := testRepo(t)
	write(filepath.Join("newdir", "a.txt"), "")
	write(filepath.Join("newdir", "b.txt"), "")
	in, keys := io.Pipe()
	defer keys.Close()
	scr := &screen{added: make(chan struct{}, 1)}
	size := func() (int, int, error) { return 100, 12, nil }
	errc := make(chan error, 1)
	go func() { errc <- runApp(config{dir: dir, interval: time.Hour}, in, scr, size, nil, nil) }()

	// The interval is an hour, so updates can only come from the toggle.
	scr.waitFor(t, "LIST  │", "newdir/", "1 untracked")
	io.WriteString(keys, "u")
	scr.waitFor(t, "LIST -u  │", "newdir/a.txt", "newdir/b.txt", "2 untracked")

	// The worktree list counts the same way, and u works there too.
	io.WriteString(keys, "w")
	scr.waitFor(t, "WORKTREES -u", "2 untracked")
	io.WriteString(keys, "u")
	scr.waitFor(t, "WORKTREES  │", "1 untracked")
	io.WriteString(keys, "w")
	scr.waitFor(t, "LIST  │", "newdir/", "1 untracked")

	io.WriteString(keys, "q")
	if err := <-errc; err != nil {
		t.Fatalf("runApp returned %v", err)
	}
}

func TestRunAppStartsInView(t *testing.T) {
	dir, _, write := testRepo(t)
	write("new.txt", "")
	in, keys := io.Pipe()
	defer keys.Close()
	scr := &screen{added: make(chan struct{}, 1)}
	size := func() (int, int, error) { return 80, 12, nil }
	cfg := config{dir: dir, interval: 20 * time.Millisecond, view: treeView}
	errc := make(chan error, 1)
	go func() { errc <- runApp(cfg, in, scr, size, nil, nil) }()

	scr.waitFor(t, "TREE", "new.txt (?)")
	io.WriteString(keys, "q")
	if err := <-errc; err != nil {
		t.Fatalf("runApp returned %v", err)
	}
}
