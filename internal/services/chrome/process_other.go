//go:build !windows

package chrome

import (
	"fmt"
	"os/exec"
)

// configureBackgroundCommand is a no-op outside Windows. The helper exists so
// the silent process call sites remain portable and explicit.
func configureBackgroundCommand(cmd *exec.Cmd) {}

func startBackgroundCommand(cmd *exec.Cmd) error {
	if cmd == nil {
		return fmt.Errorf("background command is nil")
	}
	configureBackgroundCommand(cmd)
	return cmd.Start()
}

func forgetBackgroundJob(pid int) {}
