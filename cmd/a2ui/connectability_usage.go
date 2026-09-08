//go:build unix

package main

import (
	"fmt"
	"io"
)

func printConnectabilityUsage(w io.Writer) {
	fmt.Fprint(w, `
Connectability commands:
  a2ui doctor [flags]                   Verify install, Codex MCP, daemon, and terminal readiness
  a2ui setup-codex [flags]              Register and verify the A2UI stdio MCP server in Codex

Flags for doctor:
  -server string    Daemon HTTP address (default: http://127.0.0.1:8080 or $A2UI_SERVER)
  -session string   Session ID (default: default or $A2UI_SESSION)
  -json             Output machine-readable check results

Flags for setup-codex:
  -server string    Daemon HTTP address stored in Codex MCP config
  -session string   Session ID stored in Codex MCP config
  -replace          Replace an existing MCP registration named a2ui
  -mcp-bin string   Explicit a2ui-mcp binary path (default: resolve from PATH)
  -codex-bin string Explicit Codex binary path (default: resolve from PATH)
`)
}
