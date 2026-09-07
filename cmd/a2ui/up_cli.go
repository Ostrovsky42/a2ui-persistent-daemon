//go:build unix

package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"a2ui/agentclient"
	"a2ui/ipc"
	"a2ui/localenv"
	"a2ui/supervisor"
)

type upOptions struct {
	Server       string
	Session      string
	Socket       string
	Preset       string
	ViewerPolicy string
}

func runUpCommand(ctx context.Context, args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("up", flag.ContinueOnError)
	fs.SetOutput(stderr)
	server := fs.String("server", defaultFromEnv("A2UI_SERVER", "http://127.0.0.1:8080"), "daemon HTTP URL")
	session := fs.String("session", defaultFromEnv("A2UI_SESSION", "default"), "A2UI session ID")
	socket := fs.String("socket", "", "Unix socket path")
	preset := fs.String("preset", "dashboard", "viewer preset")
	viewerPolicy := fs.String("viewer-policy", "auto", "viewer policy: auto, never, or always")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	policy, err := supervisor.ParseViewerPolicy(*viewerPolicy)
	if err != nil {
		fmt.Fprintf(stderr, "a2ui up: %v\n", err)
		return 2
	}
	serverURL := strings.TrimRight(*server, "/")
	serverAddr := strings.TrimPrefix(serverURL, "http://")
	serverAddr = strings.TrimPrefix(serverAddr, "https://")
	if *socket == "" {
		*socket = ipc.ResolveSocketPath(os.Getenv("XDG_RUNTIME_DIR"), os.Getuid())
	}
	paths, err := localenv.ResolvePaths(*socket)
	if err != nil {
		fmt.Fprintf(stderr, "a2ui up: %v\n", err)
		return 1
	}
	executable, err := os.Executable()
	if err != nil {
		fmt.Fprintf(stderr, "a2ui up: resolve executable: %v\n", err)
		return 1
	}
	a2uid, err := exec.LookPath("a2uid")
	if err != nil {
		a2uid = filepath.Join(filepath.Dir(executable), "a2uid")
	}
	options := upOptions{Server: serverURL, Session: *session, Socket: *socket, Preset: *preset, ViewerPolicy: string(policy)}
	desired := localenv.Descriptor{Version: localenv.DescriptorVersion, Socket: *socket, Server: serverURL, Session: *session, Preset: *preset, ViewerPolicy: string(policy)}
	deps := localenv.ReconcileDependencies{
		SocketLive: func(_ context.Context, path string) (bool, error) { _, err := os.Stat(path); return err == nil, nil },
		ServerLive: func(reqCtx context.Context, address string) (bool, error) { return probeServer(reqCtx, address) },
		Status: func(reqCtx context.Context, address, session string) (agentclient.Status, error) {
			return agentclient.New(address, session, &http.Client{Timeout: 2 * time.Second}).Status(reqCtx)
		},
		SupervisorRunning: func(_ context.Context, path string) (bool, error) {
			lock, err := localenv.TryAcquireSupervisorLock(path)
			if errors.Is(err, localenv.ErrSupervisorAlreadyRunning) {
				return true, nil
			}
			if err != nil {
				return false, err
			}
			return false, lock.Release()
		},
		StartDaemon: func(_ context.Context, d localenv.Descriptor, p localenv.Paths) error {
			return startDetached(detachedProcessSpec{Path: a2uid, Args: []string{"-socket", d.Socket, "-server", serverAddr, "-session", d.Session}, LogPath: p.DaemonLog, PIDPath: p.DaemonPID})
		},
		StartSupervisor: func(_ context.Context, d localenv.Descriptor, p localenv.Paths) error {
			return startDetached(detachedProcessSpec{Path: executable, Args: []string{"--supervisor", "-server", d.Server, "-session", d.Session, "-socket", d.Socket, "-preset", d.Preset, "-viewer-policy", d.ViewerPolicy, "-instance-id", d.InstanceID}, LogPath: p.SupervisorLog, PIDPath: p.SupervisorPID})
		},
	}
	result, err := localenv.Reconcile(ctx, localenv.ReconcileConfig{Paths: paths, Desired: desired}, deps)
	if err != nil {
		fmt.Fprintf(stderr, "a2ui up: %v\n", err)
		return 1
	}
	fmt.Fprintf(stdout, "A2UI ready\nsession: %s\ndaemon: running (%s)\nsupervisor: running\nviewer policy: %s\n", result.Descriptor.Session, reuseLabel(result.DaemonReused), options.ViewerPolicy)
	return 0
}

func reuseLabel(reused bool) string {
	if reused {
		return "reused"
	}
	return "started"
}

func probeServer(ctx context.Context, address string) (bool, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, strings.TrimRight(address, "/")+"/status", nil)
	if err != nil {
		return false, err
	}
	resp, err := (&http.Client{Timeout: 2 * time.Second}).Do(req)
	if err != nil {
		return false, nil
	}
	defer resp.Body.Close()
	return resp.StatusCode >= 200 && resp.StatusCode < 500, nil
}

type processViewerLauncher struct{ logPath string }

func (l processViewerLauncher) LaunchViewer(ctx context.Context, spec supervisor.ViewerSpec) error {
	return launchViewerInTerminal(ctx, spec, l.logPath)
}

func runSupervisorCommand(ctx context.Context, args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("supervisor", flag.ContinueOnError)
	fs.SetOutput(stderr)
	server := fs.String("server", "", "daemon HTTP URL")
	session := fs.String("session", "default", "A2UI session ID")
	socket := fs.String("socket", "", "Unix socket path")
	preset := fs.String("preset", "dashboard", "viewer preset")
	policyValue := fs.String("viewer-policy", "auto", "viewer policy")
	instanceID := fs.String("instance-id", "", "daemon instance identity")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	policy, err := supervisor.ParseViewerPolicy(*policyValue)
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 2
	}
	paths, err := localenv.ResolvePaths(*socket)
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}
	lock, err := localenv.TryAcquireSupervisorLock(paths.SupervisorLock)
	if err != nil {
		return 1
	}
	defer lock.Release()
	executable, err := os.Executable()
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}
	controller, err := supervisor.NewController(supervisor.ControllerConfig{Identity: supervisor.Identity{InstanceID: *instanceID, Socket: *socket, Server: *server, Session: *session}, Launcher: processViewerLauncher{logPath: paths.SupervisorLog}, Viewer: supervisor.ViewerSpec{Executable: executable, Socket: *socket, Preset: *preset}, LaunchAllowed: func() bool { return policy.Allows(os.Getenv) }})
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}
	poller, err := supervisor.NewPoller(supervisor.PollerConfig{Source: agentclient.New(*server, *session, &http.Client{Timeout: 2 * time.Second}), Controller: controller})
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}
	if err := poller.Run(ctx); err != nil && !errors.Is(err, context.Canceled) {
		fmt.Fprintln(stderr, err)
		return 1
	}
	return 0
}
