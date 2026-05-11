// Package config — dastur yo'llari va sozlamalarini boshqaradi.
//
// Windows'da dastur ma'lumotlari %APPDATA%\Cam2You\ ichida saqlanadi:
//   - app.db                 SQLite ma'lumotlar bazasi
//   - bin\ffmpeg.exe         Avtomatik o'rnatilgan FFmpeg
//   - logs\app.log           Dastur loglari
//   - logs\streams\          Stream loglari
//   - recordings\            Lokal yozuvlar (V2)
package config

import (
	"os"
	"path/filepath"
	"runtime"
)

const AppName = "Cam2You"

// Paths — dastur ishlatadigan barcha yo'llarni saqlaydi.
type Paths struct {
	// AppData — barcha foydalanuvchi ma'lumotlari (%APPDATA%\Cam2You)
	AppData string
	// FFmpegDir — auto-installer FFmpeg'ni shu yerga chiqaradi
	FFmpegDir string
	// FFmpegBin — ffmpeg.exe to'liq yo'li
	FFmpegBin string
	// FFprobeBin — ffprobe.exe to'liq yo'li (RTSP probe uchun)
	FFprobeBin string
	// DBFile — SQLite fayli
	DBFile string
	// LogsDir — barcha log fayllar
	LogsDir string
	// StreamLogsDir — har bir stream'ning alohida logi
	StreamLogsDir string
	// RecordingsDir — lokal yozuvlar (kelajak versiya)
	RecordingsDir string
	// PreviewsDir — jonli ko'rish HLS fayllari (vaqtinchalik)
	PreviewsDir string
}

// LoadPaths foydalanuvchi tizimiga mos yo'llarni qaytaradi va
// kerakli papkalarni yaratadi.
func LoadPaths() (*Paths, error) {
	root, err := appDataRoot()
	if err != nil {
		return nil, err
	}

	p := &Paths{
		AppData:       root,
		FFmpegDir:     filepath.Join(root, "bin"),
		LogsDir:       filepath.Join(root, "logs"),
		StreamLogsDir: filepath.Join(root, "logs", "streams"),
		RecordingsDir: filepath.Join(root, "recordings"),
		PreviewsDir:   filepath.Join(root, "previews"),
		DBFile:        filepath.Join(root, "app.db"),
	}

	ext := ""
	if runtime.GOOS == "windows" {
		ext = ".exe"
	}
	p.FFmpegBin = filepath.Join(p.FFmpegDir, "ffmpeg"+ext)
	p.FFprobeBin = filepath.Join(p.FFmpegDir, "ffprobe"+ext)

	// Kerakli papkalarni yarat
	for _, dir := range []string{p.AppData, p.FFmpegDir, p.LogsDir, p.StreamLogsDir, p.RecordingsDir, p.PreviewsDir} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return nil, err
		}
	}

	return p, nil
}

// appDataRoot — platformaga qarab dastur ma'lumotlar ildizini qaytaradi.
func appDataRoot() (string, error) {
	switch runtime.GOOS {
	case "windows":
		// %APPDATA% = C:\Users\<user>\AppData\Roaming
		if appdata := os.Getenv("APPDATA"); appdata != "" {
			return filepath.Join(appdata, AppName), nil
		}
	case "darwin":
		home, err := os.UserHomeDir()
		if err != nil {
			return "", err
		}
		return filepath.Join(home, "Library", "Application Support", AppName), nil
	}
	// Linux yoki APPDATA topilmasa — XDG_DATA_HOME yoki ~/.local/share
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".local", "share", AppName), nil
}
