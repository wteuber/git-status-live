//go:build !windows

package main

// enableVT is a no-op: Unix terminals understand ANSI escape sequences.
func enableVT() func() { return func() {} }
