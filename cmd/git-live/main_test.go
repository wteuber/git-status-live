package main

import (
	"reflect"
	"strings"
	"testing"
	"time"
)

func TestDecodeKeys(t *testing.T) {
	got := decodeKeys([]byte("t\tjk\x1b[A\x1b[B\x1b[5~\x1b[6~gG\x1bq"))
	want := []key{keyToggle, keyToggle, keyDown, keyUp, keyUp, keyDown, keyPageUp, keyPageDown, keyTop, keyBottom, keyQuit}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("decodeKeys = %v, want %v", got, want)
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

	a.handleKey(keyToggle)
	a.handleKey(keyBottom)
	rows = a.frame()
	if !strings.Contains(stripANSI(rows[0]), "TREE") || !strings.Contains(stripANSI(rows[3]), "both.go (M+M)") {
		t.Errorf("tree view bottom:\n%s", plain(rows))
	}
}
