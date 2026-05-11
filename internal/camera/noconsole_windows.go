//go:build windows

package camera

import (
	"os/exec"
	"syscall"
)

// hideConsoleWindow — Windows'da ffprobe subprocess CMD oynasini ochmasligi uchun.
// Wails app standartda GUI rejimida ishlaydi va child process CMD oynasini ochishi mumkin.
func hideConsoleWindow(cmd *exec.Cmd) {
	if cmd == nil {
		return
	}
	cmd.SysProcAttr = &syscall.SysProcAttr{
		HideWindow:    true,
		CreationFlags: 0x08000000, // CREATE_NO_WINDOW
	}
}
