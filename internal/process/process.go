// Package process runs external commands that terminate when their context is canceled.
package process

import (
	"context"
	"os/exec"
	"time"
)

func CommandContext(ctx context.Context, name string, args ...string) *exec.Cmd {
	cmd := exec.CommandContext(ctx, name, args...)
	configureCancellation(cmd)
	// Bound waits for inherited pipes even if a descendant detaches itself.
	cmd.WaitDelay = time.Second
	return cmd
}
