//go:build windows

package exec

import (
	osexec "os/exec"
	"syscall"
)

// detach starts the child in its own process group, detached from lean's
// console, so the emulator survives lean exiting and Ctrl-C in the terminal.
func detach(cmd *osexec.Cmd) {
	const createNewProcessGroup = 0x00000200
	const detachedProcess = 0x00000008
	cmd.SysProcAttr = &syscall.SysProcAttr{CreationFlags: createNewProcessGroup | detachedProcess}
}
