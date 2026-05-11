//go:build !windows

package ffmpeg

import "os/exec"

// hideConsoleWindow — no-op for non-Windows platforms.
func hideConsoleWindow(cmd *exec.Cmd) {}
