//go:build unix

package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
)

func defaultFromEnv(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}

func runDoctorCommand(ctx context.Context, args []string, stdout, stderr io.Writer, httpClient *http.Client) int {
	fs := flag.NewFlagSet("doctor", flag.ContinueOnError)
	fs.SetOutput(stderr)
	server := fs.String("server", defaultFromEnv("A2UI_SERVER", "http://127.0.0.1:8080"), "persistent daemon HTTP URL")
	session := fs.String("session", defaultFromEnv("A2UI_SESSION", "default"), "A2UI session ID")
	jsonOutput := fs.Bool("json", false, "print machine-readable JSON")
	if err := fs.Parse(args); err != nil {
		return 2
	}

	checks := collectDoctorChecks(ctx, doctorOptions{Server: *server, Session: *session}, httpClient)
	if *jsonOutput {
		encoder := json.NewEncoder(stdout)
		encoder.SetIndent("", "  ")
		if err := encoder.Encode(checks); err != nil {
			fmt.Fprintf(stderr, "a2ui doctor: encode report: %v\n", err)
			return 1
		}
	} else {
		for _, check := range checks {
			fmt.Fprintf(stdout, "[%s] %s: %s\n", check.Level, check.Name, check.Detail)
			if check.Fix != "" && check.Level != doctorPass {
				fmt.Fprintf(stdout, "  FIX: %s\n", check.Fix)
			}
		}
	}

	for _, check := range checks {
		if check.Level == doctorFail {
			return 1
		}
	}
	return 0
}

func runSetupCodexCommand(ctx context.Context, args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("setup-codex", flag.ContinueOnError)
	fs.SetOutput(stderr)
	server := fs.String("server", defaultFromEnv("A2UI_SERVER", "http://127.0.0.1:8080"), "persistent daemon HTTP URL")
	session := fs.String("session", defaultFromEnv("A2UI_SESSION", "default"), "A2UI session ID")
	mcpBinary := fs.String("mcp-bin", "", "path to a2ui-mcp (default: resolve from PATH)")
	codexBinary := fs.String("codex-bin", "", "path to Codex CLI (default: resolve from PATH)")
	replace := fs.Bool("replace", false, "replace an existing Codex MCP registration named a2ui")
	if err := fs.Parse(args); err != nil {
		return 2
	}

	config, err := setupCodex(ctx, setupCodexOptions{
		Server:      *server,
		Session:     *session,
		MCPBinary:   *mcpBinary,
		CodexBinary: *codexBinary,
		Replace:     *replace,
	})
	if err != nil {
		fmt.Fprintf(stderr, "a2ui setup-codex: %v\n", err)
		return 1
	}

	fmt.Fprintf(stdout, "Codex MCP server 'a2ui' registered: %s\n", config.Command)
	fmt.Fprintf(stdout, "A2UI_SERVER=%s A2UI_SESSION=%s\n", config.Server, config.Session)
	fmt.Fprintln(stdout, "Run `a2ui doctor` after starting the daemon and attaching the terminal client.")
	fmt.Fprintln(stdout, "Restart/reload Codex if it was already running, then use `/mcp` to confirm the A2UI tools are visible.")
	return 0
}
