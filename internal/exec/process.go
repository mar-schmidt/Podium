package exec

import (
	"context"
	"os/exec"
)

// Command builds an *exec.Cmd for a discovered binary with a platform-specific
// process group attached, so the whole tree (the CLI plus any children it
// spawns) can be killed together when a turn is cancelled or an agent hangs
// (R10.4). Use Kill (not cmd.Process.Kill) to terminate the group.
//
// The returned Cmd is not yet started; callers wire up stdin/stdout/stderr first.
func Command(ctx context.Context, bin string, args ...string) *exec.Cmd {
	cmd := exec.CommandContext(ctx, bin, args...)
	configureProcGroup(cmd)
	return cmd
}

// Kill terminates the process and its entire process group. It is safe to call
// on a nil process or one that has already exited.
func Kill(cmd *exec.Cmd) error {
	if cmd == nil || cmd.Process == nil {
		return nil
	}
	return killProcGroup(cmd)
}
