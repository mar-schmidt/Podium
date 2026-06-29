//go:build windows

package exec

import (
	"os/exec"
	"syscall"
)

// configureProcGroup creates a new process group on Windows so the child and its
// descendants can be terminated together.
func configureProcGroup(cmd *exec.Cmd) {
	if cmd.SysProcAttr == nil {
		cmd.SysProcAttr = &syscall.SysProcAttr{}
	}
	cmd.SysProcAttr.CreationFlags |= syscall.CREATE_NEW_PROCESS_GROUP
}

// killProcGroup terminates the process tree. Go's exec.CommandContext already
// kills the started process on context cancel; this provides an explicit kill
// path. A more thorough taskkill /T sweep can be added in Phase 9 hardening.
func killProcGroup(cmd *exec.Cmd) error {
	return cmd.Process.Kill()
}
