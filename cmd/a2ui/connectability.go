//go:build unix

package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"a2ui/agentclient"
)

type doctorLevel string

const (
	doctorPass doctorLevel = "PASS"
	doctorWarn doctorLevel = "WARN"
	doctorFail doctorLevel = "FAIL"
)

type doctorCheck struct {
	Name   string      `json:"name"`
	Level  doctorLevel `json:"level"`
	Detail string      `json:"detail"`
	Fix    string      `json:"fix,omitempty"`
}

type doctorOptions struct {
	Server  string
	Session string
}

type setupCodexOptions struct {
	Server      string
	Session     string
	MCPBinary   string
	CodexBinary string
	Replace     bool
}

type codexMCPConfig struct {
	Command string
	Server  string
	Session string
}

type codexMCPGet struct {
	Name    string `json:"name"`
	Enabled bool   `json:"enabled"`
	Transport struct {
		Type    string            `json:"type"`
		Command string            `json:"command"`
		Args    []string          `json:"args"`
		Env     map[string]string `json:"env"`
	} `json:"transport"`
}

func collectDoctorChecks(ctx context.Context, opts doctorOptions, httpClient *http.Client) []doctorCheck {
	if opts.Server == "" {
		opts.Server = "http://127.0.0.1:8080"
	}
	if opts.Session == "" {
		opts.Session = "default"
	}
	if httpClient == nil {
		httpClient = &http.Client{Timeout: 3 * time.Second}
	}

	checks := []doctorCheck{{
		Name:   "platform",
		Level:  doctorPass,
		Detail: fmt.Sprintf("%s/%s (Unix client build)", runtime.GOOS, runtime.GOARCH),
	}}

	lookup := func(name string, missing doctorLevel, fix string) (string, doctorCheck) {
		path, err := exec.LookPath(name)
		if err != nil {
			return "", doctorCheck{Name: name, Level: missing, Detail: "not found in PATH", Fix: fix}
		}
		abs, err := filepath.Abs(path)
		if err == nil {
			path = abs
		}
		return path, doctorCheck{Name: name, Level: doctorPass, Detail: path}
	}

	_, a2uidCheck := lookup("a2uid", doctorFail, "run `make install` from the A2UI repository root")
	checks = append(checks, a2uidCheck)
	mcpPath, mcpCheck := lookup("a2ui-mcp", doctorFail, "run `make install` from the A2UI repository root")
	checks = append(checks, mcpCheck)
	codexPath, codexCheck := lookup("codex", doctorFail, "install/update the Codex CLI, then rerun `a2ui doctor`")
	checks = append(checks, codexCheck)

	_, goCheck := lookup("go", doctorWarn, "Go 1.24+ is required when building A2UI from source")
	checks = append(checks, goCheck)
	_, makeCheck := lookup("make", doctorWarn, "make is required for the repository build/smoke workflow")
	checks = append(checks, makeCheck)
	for _, optional := range []struct {
		name string
		fix  string
	}{
		{"curl", "optional: install curl for HTTP smoke diagnostics"},
		{"jq", "optional: install jq for JSON smoke diagnostics"},
		{"socat", "optional: install socat for IPC smoke diagnostics"},
	} {
		_, check := lookup(optional.name, doctorWarn, optional.fix)
		checks = append(checks, check)
	}

	if codexPath == "" || mcpPath == "" {
		checks = append(checks, doctorCheck{
			Name:   "codex-mcp",
			Level:  doctorFail,
			Detail: "cannot verify Codex MCP registration until codex and a2ui-mcp are installed",
			Fix:    "install the missing binaries, then run `a2ui setup-codex`",
		})
	} else {
		config, err := readCodexMCPConfig(ctx, codexPath)
		if err != nil {
			checks = append(checks, doctorCheck{
				Name:   "codex-mcp",
				Level:  doctorFail,
				Detail: err.Error(),
				Fix:    "run `a2ui setup-codex` to register the A2UI MCP server",
			})
		} else if err := validateCodexMCPConfig(config, mcpPath, opts.Server, opts.Session); err != nil {
			checks = append(checks, doctorCheck{
				Name:   "codex-mcp",
				Level:  doctorFail,
				Detail: err.Error(),
				Fix:    "run `a2ui setup-codex --replace` with the intended server/session",
			})
		} else {
			checks = append(checks, doctorCheck{
				Name:   "codex-mcp",
				Level:  doctorPass,
				Detail: fmt.Sprintf("registered stdio server %s for %s session %q", config.Command, config.Server, config.Session),
			})
		}
	}

	a2client := agentclient.New(opts.Server, opts.Session, httpClient)
	status, err := a2client.Status(ctx)
	if err != nil {
		checks = append(checks,
			doctorCheck{Name: "daemon", Level: doctorFail, Detail: err.Error(), Fix: fmt.Sprintf("start a2uid for %s and session %q, then rerun doctor", opts.Server, opts.Session)},
			doctorCheck{Name: "terminal", Level: doctorWarn, Detail: "interactive client state is unknown because daemon is unreachable", Fix: "start the daemon first, then attach the Bubble Tea client"},
		)
		return checks
	}
	if status.Session != opts.Session {
		checks = append(checks, doctorCheck{
			Name:   "daemon",
			Level:  doctorFail,
			Detail: fmt.Sprintf("daemon reports session %q, expected %q", status.Session, opts.Session),
			Fix:    "use matching A2UI_SESSION/server settings or restart with a fresh session",
		})
	} else {
		checks = append(checks, doctorCheck{
			Name:   "daemon",
			Level:  doctorPass,
			Detail: fmt.Sprintf("reachable: revision=%d nodes=%d generation=%d pending_publish=%v", status.Revision, status.Nodes, status.Generation, status.PendingPublish),
		})
	}
	if status.HasClient {
		checks = append(checks, doctorCheck{Name: "terminal", Level: doctorPass, Detail: "interactive Bubble Tea client attached"})
	} else {
		checks = append(checks, doctorCheck{
			Name:   "terminal",
			Level:  doctorWarn,
			Detail: "daemon is reachable but no interactive terminal client is attached",
			Fix:    "attach it with `make client SOCK=<socket printed by make daemon>`",
		})
	}
	return checks
}

func setupCodex(ctx context.Context, opts setupCodexOptions) (codexMCPConfig, error) {
	if opts.Server == "" {
		opts.Server = "http://127.0.0.1:8080"
	}
	if opts.Session == "" {
		opts.Session = "default"
	}

	codexPath := opts.CodexBinary
	if codexPath == "" {
		var err error
		codexPath, err = exec.LookPath("codex")
		if err != nil {
			return codexMCPConfig{}, fmt.Errorf("codex CLI not found in PATH: %w", err)
		}
	}
	mcpPath := opts.MCPBinary
	if mcpPath == "" {
		var err error
		mcpPath, err = exec.LookPath("a2ui-mcp")
		if err != nil {
			return codexMCPConfig{}, fmt.Errorf("a2ui-mcp not found in PATH; run `make install`: %w", err)
		}
	}
	if abs, err := filepath.Abs(mcpPath); err == nil {
		mcpPath = abs
	}

	_, getErr := readCodexMCPConfig(ctx, codexPath)
	if getErr == nil {
		if !opts.Replace {
			return codexMCPConfig{}, fmt.Errorf("Codex MCP server %q is already configured; inspect it with `codex mcp get a2ui --json` or rerun with --replace", "a2ui")
		}
		if output, err := exec.CommandContext(ctx, codexPath, "mcp", "remove", "a2ui").CombinedOutput(); err != nil {
			return codexMCPConfig{}, fmt.Errorf("remove existing Codex MCP registration: %w (%s)", err, strings.TrimSpace(string(output)))
		}
	} else if !isCodexMCPNotFound(getErr) {
		return codexMCPConfig{}, fmt.Errorf("inspect existing Codex MCP registration: %w", getErr)
	}

	args := []string{
		"mcp", "add", "a2ui",
		"--env", "A2UI_SERVER=" + opts.Server,
		"--env", "A2UI_SESSION=" + opts.Session,
		"--", mcpPath,
	}
	if output, err := exec.CommandContext(ctx, codexPath, args...).CombinedOutput(); err != nil {
		return codexMCPConfig{}, fmt.Errorf("codex mcp add failed: %w (%s)", err, strings.TrimSpace(string(output)))
	}

	config, err := readCodexMCPConfig(ctx, codexPath)
	if err != nil {
		return codexMCPConfig{}, fmt.Errorf("verify Codex MCP registration: %w", err)
	}
	if err := validateCodexMCPConfig(config, mcpPath, opts.Server, opts.Session); err != nil {
		return codexMCPConfig{}, fmt.Errorf("Codex saved an unexpected MCP configuration: %w", err)
	}
	return config, nil
}

func readCodexMCPConfig(ctx context.Context, codexPath string) (codexMCPConfig, error) {
	output, err := exec.CommandContext(ctx, codexPath, "mcp", "get", "a2ui", "--json").CombinedOutput()
	if err != nil {
		text := strings.TrimSpace(string(output))
		if text == "" {
			text = err.Error()
		}
		return codexMCPConfig{}, fmt.Errorf("codex mcp get a2ui --json: %s", text)
	}
	var raw codexMCPGet
	if err := json.Unmarshal(output, &raw); err != nil {
		return codexMCPConfig{}, fmt.Errorf("decode `codex mcp get a2ui --json`: %w", err)
	}
	if raw.Name != "a2ui" {
		return codexMCPConfig{}, fmt.Errorf("Codex returned MCP server %q, expected %q", raw.Name, "a2ui")
	}
	if !raw.Enabled {
		return codexMCPConfig{}, fmt.Errorf("Codex MCP server %q is disabled", raw.Name)
	}
	if raw.Transport.Type != "stdio" {
		return codexMCPConfig{}, fmt.Errorf("Codex MCP server %q uses transport %q, expected stdio", raw.Name, raw.Transport.Type)
	}
	return codexMCPConfig{
		Command: raw.Transport.Command,
		Server:  raw.Transport.Env["A2UI_SERVER"],
		Session: raw.Transport.Env["A2UI_SESSION"],
	}, nil
}

func validateCodexMCPConfig(config codexMCPConfig, mcpPath, server, session string) error {
	configuredCommand := config.Command
	if resolved, err := exec.LookPath(configuredCommand); err == nil {
		configuredCommand = resolved
	}
	if abs, err := filepath.Abs(configuredCommand); err == nil {
		configuredCommand = abs
	}
	expectedCommand := mcpPath
	if abs, err := filepath.Abs(expectedCommand); err == nil {
		expectedCommand = abs
	}
	if filepath.Clean(configuredCommand) != filepath.Clean(expectedCommand) {
		return fmt.Errorf("Codex command is %q, expected %q", config.Command, mcpPath)
	}
	if config.Server != server {
		return fmt.Errorf("Codex A2UI_SERVER is %q, expected %q", config.Server, server)
	}
	if config.Session != session {
		return fmt.Errorf("Codex A2UI_SESSION is %q, expected %q", config.Session, session)
	}
	return nil
}

func isCodexMCPNotFound(err error) bool {
	if err == nil {
		return false
	}
	text := strings.ToLower(err.Error())
	return strings.Contains(text, "no mcp server named") || strings.Contains(text, "not found")
}
