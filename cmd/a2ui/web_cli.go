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
	"strconv"
	"syscall"
	"time"

	airweb "a2ui/adapter/web"
	"a2ui/ipc"
	"a2ui/protocol"
)

func validateWebListenAddress(addr string) error {
	host, port, err := net.SplitHostPort(addr)
	if err != nil {
		return fmt.Errorf("invalid listen address %q: %w", addr, err)
	}
	ip := net.ParseIP(host)
	if ip == nil || !ip.IsLoopback() {
		return fmt.Errorf("web renderer listen address must use a numeric loopback IP")
	}
	p, err := strconv.ParseUint(port, 10, 16)
	if err != nil {
		return fmt.Errorf("invalid listen port %q", port)
	}
	if p > 65535 {
		return fmt.Errorf("invalid listen port %q", port)
	}
	return nil
}

func webRendererURL(addr string) string {
	return "http://" + addr + "/"
}

func runWeb(args []string) {
	fs := flag.NewFlagSet("a2ui web", flag.ExitOnError)
	socket := fs.String("socket", "", "Unix socket path (default: $XDG_RUNTIME_DIR/a2ui/a2ui.sock)")
	listen := fs.String("listen", "127.0.0.1:0", "Loopback HTTP listen address")
	_ = fs.Parse(args)

	if err := validateWebListenAddress(*listen); err != nil {
		fmt.Fprintf(os.Stderr, "a2ui web: %v\n", err)
		os.Exit(2)
	}

	path := resolveClientSocket(*socket, os.Getenv("XDG_RUNTIME_DIR"), os.Getuid())
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	client, perr := ipc.Dial(ctx, path, ipc.RecordLimit(protocol.DefaultLimits()))
	cancel()
	if perr != nil {
		fmt.Fprintf(os.Stderr, "a2ui web: %s\n", perr)
		os.Exit(1)
	}
	defer client.Close()

	ln, err := net.Listen("tcp", *listen)
	if err != nil {
		fmt.Fprintf(os.Stderr, "a2ui web: listen: %v\n", err)
		os.Exit(1)
	}
	defer ln.Close()

	server := &http.Server{
		Handler:           airweb.NewHandler(client),
		ReadHeaderTimeout: 5 * time.Second,
	}

	url := webRendererURL(ln.Addr().String())
	fmt.Fprintf(os.Stdout, "AIR Web renderer: %s\n", url)
	fmt.Fprintln(os.Stdout, "Press Ctrl+C to detach the Web renderer.")

	errCh := make(chan error, 1)
	go func() {
		errCh <- server.Serve(ln)
	}()

	stopCtx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	select {
	case <-stopCtx.Done():
		shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer shutdownCancel()
		if err := server.Shutdown(shutdownCtx); err != nil {
			fmt.Fprintf(os.Stderr, "a2ui web: shutdown: %v\n", err)
			os.Exit(1)
		}
	case err := <-errCh:
		if err != nil && !errors.Is(err, http.ErrServerClosed) {
			fmt.Fprintf(os.Stderr, "a2ui web: serve: %v\n", err)
			os.Exit(1)
		}
	}
}
