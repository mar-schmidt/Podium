//go:build windows

package exec

import (
	"os/exec"
	"strconv"
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

// killProcGroup terminates the whole process tree rooted at the child. The
// claude/codex CLIs are npm .cmd shims that spawn node, which in turn spawns
// further children, so killing only the shim (cmd.Process.Kill) would orphan
// the real worker. `taskkill /T` walks and kills the entire tree by PID; we
// fall back to a direct kill if taskkill is unavailable or the tree already
// exited. This mirrors the Unix negative-PID group kill (R10.4).
func killProcGroup(cmd *exec.Cmd) error {
	pid := cmd.Process.Pid
	tk := exec.Command("taskkill", "/T", "/F", "/PID", strconv.Itoa(pid))
	tk.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}
	if err := tk.Run(); err != nil {
		// taskkill missing, or the process is already gone — kill what we can.
		return cmd.Process.Kill()
	}
	return nil
}
