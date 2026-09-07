//go:build linux

package main

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"a2ui/supervisor"
)

func TestProcessViewerLauncherUsesSupportedTerminalBackends(t *testing.T) {
	cases := []struct {
		name       string
		terminal   string
		wantPrefix []string
	}{
		{name: "xdg terminal exec", terminal: "xdg-terminal-exec", wantPrefix: []string{"--"}},
		{name: "gnome terminal", terminal: "gnome-terminal", wantPrefix: []string{"--"}},
		{name: "kitty", terminal: "kitty"},
		{name: "alacritty", terminal: "alacritty", wantPrefix: []string{"-e"}},
		{name: "konsole", terminal: "konsole", wantPrefix: []string{"-e"}},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			marker := filepath.Join(dir, "terminal-argv.txt")
			terminal := filepath.Join(dir, tc.terminal)
			script := "#!/bin/sh\nprintf '%s\\n' \"$@\" > \"$A2UI_TEST_MARKER\"\n"
			if err := os.WriteFile(terminal, []byte(script), 0o700); err != nil {
				t.Fatalf("write fake terminal: %v", err)
			}
			t.Setenv("PATH", dir)
			t.Setenv("A2UI_TEST_MARKER", marker)

			launcher := processViewerLauncher{logPath: filepath.Join(dir, "supervisor.log")}
			spec := supervisor.ViewerSpec{
				Executable: "/opt/a2ui/bin/a2ui",
				Socket:     filepath.Join(dir, "a2ui.sock"),
				Preset:     "dashboard",
			}
			if err := launcher.LaunchViewer(context.Background(), spec); err != nil {
				t.Fatalf("LaunchViewer: %v", err)
			}

			deadline := time.Now().Add(time.Second)
			var body []byte
			for time.Now().Before(deadline) {
				var err error
				body, err = os.ReadFile(marker)
				if err == nil {
					break
				}
				time.Sleep(10 * time.Millisecond)
			}
			if len(body) == 0 {
				t.Fatal("terminal backend did not execute")
			}

			got := strings.Fields(string(body))
			want := append([]string{}, tc.wantPrefix...)
			want = append(want, spec.Executable, "-socket", spec.Socket, "-preset", spec.Preset)
			if len(got) != len(want) {
				t.Fatalf("terminal argv=%q, want %q", got, want)
			}
			for i := range want {
				if got[i] != want[i] {
					t.Fatalf("terminal argv[%d]=%q, want %q; full argv=%q", i, got[i], want[i], got)
				}
			}
		})
	}
}

func TestProcessViewerLauncherHonorsDiscoveryOrder(t *testing.T) {
	dir := t.TempDir()
	marker := filepath.Join(dir, "terminal-name.txt")
	for _, terminal := range []string{"xdg-terminal-exec", "gnome-terminal", "kitty", "alacritty", "konsole"} {
		path := filepath.Join(dir, terminal)
		script := "#!/bin/sh\nprintf '%s\\n' \"$(basename \"$0\")\" > \"$A2UI_TEST_MARKER\"\n"
		if err := os.WriteFile(path, []byte(script), 0o700); err != nil {
			t.Fatalf("write fake terminal %s: %v", terminal, err)
		}
	}
	t.Setenv("PATH", dir)
	t.Setenv("A2UI_TEST_MARKER", marker)

	launcher := processViewerLauncher{logPath: filepath.Join(dir, "supervisor.log")}
	if err := launcher.LaunchViewer(context.Background(), supervisor.ViewerSpec{
		Executable: "/opt/a2ui/bin/a2ui",
		Socket:     filepath.Join(dir, "a2ui.sock"),
		Preset:     "dashboard",
	}); err != nil {
		t.Fatalf("LaunchViewer: %v", err)
	}

	deadline := time.Now().Add(time.Second)
	var body []byte
	for time.Now().Before(deadline) {
		var err error
		body, err = os.ReadFile(marker)
		if err == nil {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if got := strings.TrimSpace(string(body)); got != "xdg-terminal-exec" {
		t.Fatalf("selected terminal=%q, want xdg-terminal-exec", got)
	}
}
