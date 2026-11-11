//go:build unix

package runner

import (
	"os/exec"
	"syscall"
)

// groupKill runs cmd in its own process group and kills the whole group
// when its context ends, so children (go run's binary, dotnet's build)
// die with it.
func groupKill(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	cmd.Cancel = func() error {
		if cmd.Process == nil {
			return nil
		}
		return syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
	}
}
