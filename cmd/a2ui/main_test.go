//go:build unix

package main

import "testing"

func TestResolveClientSocketMatchesDaemonConvention(t *testing.T) {
	if got := resolveClientSocket("", "/run/user/1000", 1000); got != "/run/user/1000/a2ui/a2ui.sock" {
		t.Fatalf("socket=%q", got)
	}
}
