//go:build !windows

package claude

import "os/exec"

func setPlatformProcAttr(cmd *exec.Cmd) {}
