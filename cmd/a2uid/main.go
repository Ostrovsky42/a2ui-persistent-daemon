//go:build unix

package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"a2ui/daemon"
	"a2ui/ipc"
	"a2ui/protocol"
)

func resolveDaemonSocket(explicit, xdg string, uid int) string {
	if explicit != "" {
		return explicit
	}
	return ipc.ResolveSocketPath(xdg, uid)
}

func closeDaemonListener(ln *net.UnixListener) {
	if ln != nil {
		_ = ln.Close()
	}
}

func main() {
	var (
		socket  = flag.String("socket", "", "Unix socket path (default: $XDG_RUNTIME_DIR/a2ui/a2ui.sock)")
		server  = flag.String("server", "", "optional MCP HTTP listen address, e.g. 127.0.0.1:8080")
		session = flag.String("session", "default", "A2UI agent session identifier")
	)
	flag.Parse()

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	path := resolveDaemonSocket(*socket, os.Getenv("XDG_RUNTIME_DIR"), os.Getuid())
	ln, perr := ipc.ListenUnix(path)
	if perr != nil {
		fmt.Fprintf(os.Stderr, "a2uid: %s\n", perr)
		os.Exit(1)
	}
	// The UnixListener owns unlink-on-close. Do not remove the path by name:
	// after Close a new daemon may already have bound the same pathname.
	defer closeDaemonListener(ln)

	runtimeServer := ""
	if *server != "" {
		runtimeServer = "http://" + *server
	}
	d := daemon.NewWithRuntimeIdentity(*session, protocol.DefaultLimits(), nil, path, runtimeServer)
	errCh := make(chan error, 2)
	go func() { errCh <- d.Serve(ctx, ln) }()

	var httpServer *http.Server
	if *server != "" {
		httpServer = &http.Server{
			Addr:              *server,
			Handler:           d,
			ReadHeaderTimeout: 5 * time.Second,
		}
		go func() {
			err := httpServer.ListenAndServe()
			if errors.Is(err, http.ErrServerClosed) {
				err = nil
			}
			errCh <- err
		}()
	}

	fmt.Fprintf(os.Stderr, "a2uid: ipc=%s", path)
	if *server != "" {
		fmt.Fprintf(os.Stderr, " mcp=http://%s", *server)
	}
	fmt.Fprintln(os.Stderr)

	select {
	case <-ctx.Done():
	case err := <-errCh:
		if err != nil && !errors.Is(err, net.ErrClosed) {
			fmt.Fprintf(os.Stderr, "a2uid: %v\n", err)
			stop()
		}
	}

	if httpServer != nil {
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		_ = httpServer.Shutdown(shutdownCtx)
		cancel()
	}
	stop()
	closeDaemonListener(ln)
}
