//go:build !windows

package camera

import "os/exec"

func hideConsoleWindow(cmd *exec.Cmd) {}
