//go:build !unix && !windows

package exec

import osexec "os/exec"

func detach(*osexec.Cmd) {}
