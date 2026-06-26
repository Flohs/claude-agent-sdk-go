//go:build windows

package claude

import (
	"os/exec"
	"syscall"
)

// setPlatformProcAttr suppresses the brief console-window flash that occurs on
// Windows when a new process is spawned without explicit process-creation flags.
// Port of TypeScript SDK v0.3.193.
func setPlatformProcAttr(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}
}
