//go:build !windows

package exec

import (
	"os/exec"
	"syscall"
)

// configureProcGroup puts the child in its own process group so we can signal the
// whole group (the CLI and any descendants) at once.
func configureProcGroup(cmd *exec.Cmd) {
	if cmd.SysProcAttr == nil {
		cmd.SysProcAttr = &syscall.SysProcAttr{}
	}
	cmd.SysProcAttr.Setpgid = true
}

// killProcGroup sends SIGKILL to the negative PID, which targets the entire
// process group rooted at the child.
func killProcGroup(cmd *exec.Cmd) error {
	pgid, err := syscall.Getpgid(cmd.Process.Pid)
	if err != nil {
		// Fall back to killing just the process if the group can't be resolved
		// (e.g. it already exited).
		return cmd.Process.Kill()
	}
	return syscall.Kill(-pgid, syscall.SIGKILL)
}
