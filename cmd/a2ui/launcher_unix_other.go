//go:build unix && !linux

package main

import (
	"context"
	"fmt"

	"a2ui/supervisor"
)

func launchViewerInTerminal(_ context.Context, _ supervisor.ViewerSpec, _ string) error {
	return fmt.Errorf("automatic terminal viewer launch is currently supported on Linux only")
}
