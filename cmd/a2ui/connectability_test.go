package main

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writeExecutable(t *testing.T, dir, name, body string) string {
	t.Helper()
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, []byte(body), 0o755); err != nil {
		t.Fatal(err)
	}
	return path
}

func checkByName(t *testing.T, checks []doctorCheck, name string) doctorCheck {
	t.Helper()
	for _, check := range checks {
		if check.Name == name {
			return check
		}
	}
	t.Fatalf("doctor check %q not found in %#v", name, checks)
	return doctorCheck{}
}

func TestDoctorVerifiesCodexRegistrationDaemonAndTerminal(t *testing.T) {
	bin := t.TempDir()
	for _, name := range []string{"a2uid", "go", "make"} {
		writeExecutable(t, bin, name, "#!/bin/sh\nexit 0\n")
	}
	mcpPath := writeExecutable(t, bin, "a2ui-mcp", "#!/bin/sh\nexit 0\n")

	daemon := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/status" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"session":"doctor-session","revision":3,"nodes":2,"has_client":true,"generation":4,"pending_publish":false}`)
	}))
	defer daemon.Close()

	codexBody := fmt.Sprintf(`#!/bin/sh
if [ "$1" = "mcp" ] && [ "$2" = "get" ] && [ "$3" = "a2ui" ] && [ "$4" = "--json" ]; then
  printf '%%s\n' '{"name":"a2ui","enabled":true,"transport":{"type":"stdio","command":"%s","args":[],"env":{"A2UI_SERVER":"%s","A2UI_SESSION":"doctor-session"}},"enabled_tools":null,"disabled_tools":null,"startup_timeout_sec":null,"tool_timeout_sec":null}'
  exit 0
fi
exit 2
`, mcpPath, daemon.URL)
	writeExecutable(t, bin, "codex", codexBody)
	t.Setenv("PATH", bin)

	checks := collectDoctorChecks(context.Background(), doctorOptions{
		Server:  daemon.URL,
		Session: "doctor-session",
	}, daemon.Client())

	for _, name := range []string{"platform", "a2uid", "a2ui-mcp", "codex", "codex-mcp", "daemon", "terminal"} {
		check := checkByName(t, checks, name)
		if check.Level != doctorPass {
			t.Fatalf("check %q = %#v, want PASS", name, check)
		}
	}
	for _, name := range []string{"curl", "jq", "socat"} {
		check := checkByName(t, checks, name)
		if check.Level != doctorWarn {
			t.Fatalf("optional check %q = %#v, want WARN", name, check)
		}
	}
}

func TestDoctorExplainsMissingCodexRegistration(t *testing.T) {
	bin := t.TempDir()
	for _, name := range []string{"a2uid", "a2ui-mcp", "go", "make"} {
		writeExecutable(t, bin, name, "#!/bin/sh\nexit 0\n")
	}
	writeExecutable(t, bin, "codex", "#!/bin/sh\necho \"No MCP server named 'a2ui' found.\" >&2\nexit 1\n")
	t.Setenv("PATH", bin)

	daemon := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `{"session":"default","has_client":false}`)
	}))
	defer daemon.Close()

	checks := collectDoctorChecks(context.Background(), doctorOptions{Server: daemon.URL, Session: "default"}, daemon.Client())
	check := checkByName(t, checks, "codex-mcp")
	if check.Level != doctorFail {
		t.Fatalf("codex-mcp check = %#v, want FAIL", check)
	}
	if !strings.Contains(check.Fix, "a2ui setup-codex") {
		t.Fatalf("codex-mcp fix = %q, want setup command", check.Fix)
	}
	terminal := checkByName(t, checks, "terminal")
	if terminal.Level != doctorWarn || !strings.Contains(terminal.Fix, "client") {
		t.Fatalf("terminal check = %#v, want actionable WARN", terminal)
	}
}

func TestSetupCodexUsesCodexCLIAndVerifiesResult(t *testing.T) {
	bin := t.TempDir()
	mcpPath := writeExecutable(t, bin, "a2ui-mcp", "#!/bin/sh\nexit 0\n")
	statePath := filepath.Join(t.TempDir(), "configured")
	argsPath := filepath.Join(t.TempDir(), "args")
	serverURL := "http://127.0.0.1:18080"

	codexBody := fmt.Sprintf(`#!/bin/sh
state=%q
args_file=%q
if [ "$1" = "mcp" ] && [ "$2" = "get" ]; then
  if [ ! -f "$state" ]; then
    echo "No MCP server named 'a2ui' found." >&2
    exit 1
  fi
  printf '%%s\n' '{"name":"a2ui","enabled":true,"transport":{"type":"stdio","command":"%s","args":[],"env":{"A2UI_SERVER":"%s","A2UI_SESSION":"setup-session"}}}'
  exit 0
fi
if [ "$1" = "mcp" ] && [ "$2" = "add" ]; then
  printf '%%s\n' "$*" > "$args_file"
  touch "$state"
  exit 0
fi
exit 2
`, statePath, argsPath, mcpPath, serverURL)
	codexPath := writeExecutable(t, bin, "codex", codexBody)

	config, err := setupCodex(context.Background(), setupCodexOptions{
		Server:      serverURL,
		Session:     "setup-session",
		MCPBinary:   mcpPath,
		CodexBinary: codexPath,
	})
	if err != nil {
		t.Fatalf("setupCodex: %v", err)
	}
	if config.Command != mcpPath || config.Server != serverURL || config.Session != "setup-session" {
		t.Fatalf("verified config = %#v", config)
	}
	args, err := os.ReadFile(argsPath)
	if err != nil {
		t.Fatal(err)
	}
	got := string(args)
	for _, want := range []string{
		"mcp add a2ui",
		"--env A2UI_SERVER=" + serverURL,
		"--env A2UI_SESSION=setup-session",
		"-- " + mcpPath,
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("codex add args = %q, missing %q", got, want)
		}
	}
}

func TestSetupCodexRefusesExistingRegistrationWithoutReplace(t *testing.T) {
	bin := t.TempDir()
	mcpPath := writeExecutable(t, bin, "a2ui-mcp", "#!/bin/sh\nexit 0\n")
	codexBody := fmt.Sprintf(`#!/bin/sh
if [ "$1" = "mcp" ] && [ "$2" = "get" ]; then
  printf '%%s\n' '{"name":"a2ui","enabled":true,"transport":{"type":"stdio","command":"%s","args":[],"env":{"A2UI_SERVER":"http://127.0.0.1:8080","A2UI_SESSION":"old"}}}'
  exit 0
fi
exit 2
`, mcpPath)
	codexPath := writeExecutable(t, bin, "codex", codexBody)

	_, err := setupCodex(context.Background(), setupCodexOptions{
		Server:      "http://127.0.0.1:8080",
		Session:     "new",
		MCPBinary:   mcpPath,
		CodexBinary: codexPath,
	})
	if err == nil || !strings.Contains(err.Error(), "already configured") || !strings.Contains(err.Error(), "--replace") {
		t.Fatalf("setupCodex error = %v, want safe overwrite refusal", err)
	}
}

func TestSetupCodexDoesNotTreatUnrelatedNotFoundAsMissingRegistration(t *testing.T) {
	bin := t.TempDir()
	mcpPath := writeExecutable(t, bin, "a2ui-mcp", "#!/bin/sh\nexit 0\n")
	addMarker := filepath.Join(t.TempDir(), "add-called")
	codexBody := fmt.Sprintf(`#!/bin/sh
if [ "$1" = "mcp" ] && [ "$2" = "get" ]; then
  echo "failed to load configuration: profile not found" >&2
  exit 1
fi
if [ "$1" = "mcp" ] && [ "$2" = "add" ]; then
  touch %q
  exit 0
fi
exit 2
`, addMarker)
	codexPath := writeExecutable(t, bin, "codex", codexBody)

	_, err := setupCodex(context.Background(), setupCodexOptions{
		Server:      "http://127.0.0.1:8080",
		Session:     "safe",
		MCPBinary:   mcpPath,
		CodexBinary: codexPath,
	})
	if err == nil || !strings.Contains(err.Error(), "inspect existing Codex MCP registration") {
		t.Fatalf("setupCodex error = %v, want inspection failure", err)
	}
	if _, statErr := os.Stat(addMarker); !os.IsNotExist(statErr) {
		t.Fatalf("codex mcp add was called after unrelated inspection error")
	}
}
