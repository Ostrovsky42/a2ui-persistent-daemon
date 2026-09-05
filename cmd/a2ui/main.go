//go:build unix

package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"time"

	a2tea "a2ui/adapter/bubbletea"
	"a2ui/ipc"
	tea "github.com/charmbracelet/bubbletea"
)

func resolveClientSocket(explicit, xdg string, uid int) string {
	if explicit != "" {
		return explicit
	}
	return ipc.ResolveSocketPath(xdg, uid)
}

func main() {
	var (
		socket     = flag.String("socket", "", "Unix socket path (default: $XDG_RUNTIME_DIR/a2ui/a2ui.sock)")
		presetName = flag.String("preset", "dashboard", "presentation preset: minimal, dashboard, or dense")
	)
	flag.Parse()

	preset, err := a2tea.ParsePreset(*presetName)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(2)
	}
	path := resolveClientSocket(*socket, os.Getenv("XDG_RUNTIME_DIR"), os.Getuid())
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	client, perr := ipc.Dial(ctx, path, ipc.DefaultMaxMessageBytes)
	cancel()
	if perr != nil {
		fmt.Fprintf(os.Stderr, "a2ui: %s\n", perr)
		os.Exit(1)
	}
	defer client.Close()

	model := a2tea.NewModelWithController(client, a2tea.DefaultTheme, preset, client.Updates())
	program := tea.NewProgram(model, tea.WithAltScreen())
	if _, err := program.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "a2ui: %v\n", err)
		os.Exit(1)
	}
}
