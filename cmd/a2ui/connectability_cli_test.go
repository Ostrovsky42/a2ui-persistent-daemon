package main

import (
	"bytes"
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRunDoctorCommandReportsActionableFailure(t *testing.T) {
	bin := t.TempDir()
	for _, name := range []string{"a2uid", "a2ui-mcp", "go", "make"} {
		writeExecutable(t, bin, name, "#!/bin/sh\nexit 0\n")
	}
	t.Setenv("PATH", bin)

	daemon := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `{"session":"cli-session","has_client":false}`)
	}))
	defer daemon.Close()

	var stdout, stderr bytes.Buffer
	code := runDoctorCommand(context.Background(), []string{
		"--server", daemon.URL,
		"--session", "cli-session",
	}, &stdout, &stderr, daemon.Client())
	if code != 1 {
		t.Fatalf("doctor exit = %d, want 1", code)
	}
	got := stdout.String()
	if !strings.Contains(got, "[FAIL] codex:") || !strings.Contains(got, "[WARN] terminal:") || !strings.Contains(got, "FIX:") {
		t.Fatalf("doctor output = %q, want actionable FAIL/WARN report", got)
	}
	if stderr.Len() != 0 {
		t.Fatalf("doctor stderr = %q, want empty", stderr.String())
	}
}

func TestRunDoctorCommandJSONIsMachineReadable(t *testing.T) {
	bin := t.TempDir()
	for _, name := range []string{"a2uid", "a2ui-mcp", "go", "make"} {
		writeExecutable(t, bin, name, "#!/bin/sh\nexit 0\n")
	}
	t.Setenv("PATH", bin)

	daemon := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `{"session":"json-session","has_client":false}`)
	}))
	defer daemon.Close()

	var stdout, stderr bytes.Buffer
	_ = runDoctorCommand(context.Background(), []string{
		"--server", daemon.URL,
		"--session", "json-session",
		"--json",
	}, &stdout, &stderr, daemon.Client())
	if !strings.HasPrefix(strings.TrimSpace(stdout.String()), "[") || !strings.Contains(stdout.String(), `"name": "daemon"`) {
		t.Fatalf("doctor JSON = %q", stdout.String())
	}
	if stderr.Len() != 0 {
		t.Fatalf("doctor stderr = %q, want empty", stderr.String())
	}
}

func TestRunSetupCodexCommandRegistersThroughCodexCLI(t *testing.T) {
	bin := t.TempDir()
	mcpPath := writeExecutable(t, bin, "a2ui-mcp", "#!/bin/sh\nexit 0\n")
	statePath := filepath.Join(t.TempDir(), "configured")
	serverURL := "http://127.0.0.1:18081"
	codexBody := fmt.Sprintf(`#!/bin/sh
state=%q
if [ "$1" = "mcp" ] && [ "$2" = "get" ]; then
  if [ ! -f "$state" ]; then
    echo "No MCP server named 'a2ui' found." >&2
    exit 1
  fi
  printf '%%s\n' '{"name":"a2ui","enabled":true,"transport":{"type":"stdio","command":"%s","args":[],"env":{"A2UI_SERVER":"%s","A2UI_SESSION":"cli-session"}}}'
  exit 0
fi
if [ "$1" = "mcp" ] && [ "$2" = "add" ]; then
  touch "$state"
  exit 0
fi
exit 2
`, statePath, mcpPath, serverURL)
	codexPath := writeExecutable(t, bin, "codex", codexBody)

	var stdout, stderr bytes.Buffer
	code := runSetupCodexCommand(context.Background(), []string{
		"--server", serverURL,
		"--session", "cli-session",
		"--mcp-bin", mcpPath,
		"--codex-bin", codexPath,
	}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("setup-codex exit = %d stdout=%q stderr=%q", code, stdout.String(), stderr.String())
	}
	if !strings.Contains(stdout.String(), "Codex MCP server 'a2ui' registered") || !strings.Contains(stdout.String(), "a2ui doctor") || !strings.Contains(stdout.String(), "/mcp") {
		t.Fatalf("setup-codex output = %q, want verification guidance", stdout.String())
	}
	if stderr.Len() != 0 {
		t.Fatalf("setup-codex stderr = %q, want empty", stderr.String())
	}
	if _, err := os.Stat(statePath); err != nil {
		t.Fatalf("Codex registration state missing: %v", err)
	}
}
