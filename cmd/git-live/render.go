package main

import (
	"strings"
	"unicode/utf8"
)

const (
	ansiReset   = "\x1b[0m"
	ansiRed     = "\x1b[31m"
	ansiGreen   = "\x1b[32m"
	ansiBlue    = "\x1b[1;34m"
	ansiDim     = "\x1b[2m"
	ansiReverse = "\x1b[7m"
)

func red(s string) string   { return ansiRed + s + ansiReset }
func green(s string) string { return ansiGreen + s + ansiReset }
func blue(s string) string  { return ansiBlue + s + ansiReset }
func dim(s string) string   { return ansiDim + s + ansiReset }

// stripANSI removes escape sequences, leaving only visible text.
func stripANSI(s string) string {
	var b strings.Builder
	for i := 0; i < len(s); i++ {
		if s[i] == '\x1b' {
			i = skipEscape(s, i)
			continue
		}
		b.WriteByte(s[i])
	}
	return b.String()
}

// skipEscape returns the index of the last byte of the CSI sequence at s[i].
func skipEscape(s string, i int) int {
	if i+1 < len(s) && s[i+1] == '[' {
		for j := i + 2; j < len(s); j++ {
			if s[j] >= 0x40 && s[j] <= 0x7e {
				return j
			}
		}
		return len(s) - 1
	}
	return i
}

func visibleLen(s string) int { return utf8.RuneCountInString(stripANSI(s)) }

// truncate cuts s to at most width visible runes, keeping escape sequences.
func truncate(s string, width int) string {
	if visibleLen(s) <= width {
		return s
	}
	var b strings.Builder
	n := 0
	for i := 0; i < len(s); {
		if s[i] == '\x1b' {
			j := skipEscape(s, i)
			b.WriteString(s[i : j+1])
			i = j + 1
			continue
		}
		if n == width {
			break
		}
		_, size := utf8.DecodeRuneInString(s[i:])
		b.WriteString(s[i : i+size])
		i += size
		n++
	}
	b.WriteString(ansiReset)
	return b.String()
}

// pad right-pads s with spaces to width visible runes.
func pad(s string, width int) string {
	if n := visibleLen(s); n < width {
		return s + strings.Repeat(" ", width-n)
	}
	return s
}
