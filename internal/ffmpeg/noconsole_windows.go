//go:build windows

package ffmpeg

import (
	"os/exec"
	"syscall"
)

// hideConsoleWindow — Windows'da subprocess ishga tushganda konsol oynasi paydo bo'lmasligi uchun.
//
// Go'da exec.Command ishga tushirsa, child process default holda CMD oynasini ochadi
// (agar bu GUI dastur emas, yoki Process inherits stdio). FFmpeg/ffprobe bu kategoriyada.
//
// SysProcAttr.HideWindow = true va CreationFlags = CREATE_NO_WINDOW kombinatsiyasi
// konsol oynasini butunlay ko'rsatmaydi.
func hideConsoleWindow(cmd *exec.Cmd) {
	if cmd == nil {
		return
	}
	cmd.SysProcAttr = &syscall.SysProcAttr{
		HideWindow:    true,
		CreationFlags: 0x08000000, // CREATE_NO_WINDOW
	}
}
