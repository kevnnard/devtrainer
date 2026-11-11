//go:build !unix

package runner

import "os/exec"

// groupKill kills only the process itself where process groups are not
// available.
func groupKill(cmd *exec.Cmd) {}
