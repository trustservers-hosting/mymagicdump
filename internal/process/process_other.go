//go:build !linux && !darwin

package process

import "os/exec"

func configureCancellation(cmd *exec.Cmd) {
	// Use CommandContext's default process cancellation on other platforms.
}
