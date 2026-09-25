//go:build unix

package exec

import (
	osexec "os/exec"
	"syscall"
)

// detach puts the child in its own session so it outlives lean.
func detach(cmd *osexec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
}
