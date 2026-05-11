// Package ffmpeg — FFmpeg auto-installer va process boshqaruvi.
//
// Installer Windows uchun gyan.dev rasmiy build'larini yuklaydi:
// https://www.gyan.dev/ffmpeg/builds/ffmpeg-release-essentials.zip
//
// Foydalanuvchi tajribasi:
//  1. App ishga tushadi → Find() chaqiriladi
//  2. FFmpeg topilmasa → progress bilan EnsureInstalled() ishga tushadi
//  3. UI'ga progress event'lar yuboriladi
//  4. Tamom — FFmpeg ishga tayyor
package ffmpeg

import (
	"archive/zip"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

// mirrors — Windows x64 uchun FFmpeg yuklash manbalari.
// Tartib muhim: birinchisi sinab ko'riladi, ishlamasa keyingisiga o'tadi.
//
// Manbalar haqida:
//   - GitHub BtbN — GitHub CDN, dunyo bo'ylab tez (~150MB, GPL bilan to'liq)
//   - gyan.dev    — ixcham essentials build (~80MB), lekin server o'rta tezlikda
var mirrors = []mirror{
	{
		Name: "GitHub (BtbN) — tez CDN",
		URL:  "https://github.com/BtbN/FFmpeg-Builds/releases/latest/download/ffmpeg-master-latest-win64-lgpl.zip",
	},
	{
		Name: "gyan.dev — ixcham build",
		URL:  "https://www.gyan.dev/ffmpeg/builds/ffmpeg-release-essentials.zip",
	},
}

type mirror struct {
	Name string
	URL  string
}

// ProgressFunc — yuklash va o'rnatish jarayonida UI'ga xabar berish uchun.
// stage: "downloading" | "extracting" | "done"
// percent: 0-100, agar noma'lum bo'lsa -1
// speedMBps: hozirgi yuklash tezligi (MB/s), agar mavjud bo'lmasa 0
// etaSec: taxminiy qolgan vaqt (sekund), 0 — noma'lum
type ProgressFunc func(stage string, percent float64, speedMBps float64, etaSec int, message string)

// Installer — FFmpeg'ni topish va o'rnatish vazifalari.
type Installer struct {
	FFmpegBin  string // kutilayotgan ffmpeg yo'li (config.Paths'dan keladi)
	FFprobeBin string // kutilayotgan ffprobe yo'li
	BinDir     string // qaerga chiqarish kerak (config.Paths.FFmpegDir)
}

// New — yangi installer yaratadi.
func New(ffmpegBin, ffprobeBin, binDir string) *Installer {
	return &Installer{
		FFmpegBin:  ffmpegBin,
		FFprobeBin: ffprobeBin,
		BinDir:     binDir,
	}
}

// Find — FFmpeg'ni quyidagi tartibda qidiradi:
//  1. Dastur o'z papkasida (BinDir/ffmpeg.exe)
//  2. Sistema PATH'da
//
// Topilsa, ishlatish kerak bo'lgan to'liq yo'lni qaytaradi. Topilmasa "" qaytadi.
func (i *Installer) Find() (ffmpegPath, ffprobePath string) {
	// 1) Avval o'z papkamizdan qaraymiz (boshqarish oson)
	if fileExists(i.FFmpegBin) && fileExists(i.FFprobeBin) {
		return i.FFmpegBin, i.FFprobeBin
	}

	// 2) Sistema PATH'da qidiramiz
	if sysPath, err := exec.LookPath("ffmpeg"); err == nil {
		probePath, _ := exec.LookPath("ffprobe")
		return sysPath, probePath
	}

	return "", ""
}

// IsInstalled — FFmpeg topilganmi yoki yo'qmi.
func (i *Installer) IsInstalled() bool {
	bin, _ := i.Find()
	return bin != ""
}

// VerifyVersion — ffmpeg.exe ni ishga tushirib, versiyani qaytaradi.
// Buzilgan fayl topilsa, error qaytaradi.
func (i *Installer) VerifyVersion(ctx context.Context) (string, error) {
	bin, _ := i.Find()
	if bin == "" {
		return "", errors.New("ffmpeg topilmadi")
	}
	cmd := exec.CommandContext(ctx, bin, "-version")
	out, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("ffmpeg -version xatoligi: %w", err)
	}
	// Birinchi qator: "ffmpeg version N.N ..."
	lines := strings.SplitN(string(out), "\n", 2)
	return strings.TrimSpace(lines[0]), nil
}

// EnsureInstalled — agar FFmpeg yo'q bo'lsa, yuklaydi va chiqaradi.
// Birinchi manba ishlamasa, keyingisiga avtomatik o'tadi.
//
// Hozircha faqat Windows x64 qo'llab-quvvatlanadi.
func (i *Installer) EnsureInstalled(ctx context.Context, progress ProgressFunc) error {
	if i.IsInstalled() {
		if progress != nil {
			progress("done", 100, 0, 0, "FFmpeg allaqachon o'rnatilgan")
		}
		return nil
	}

	if runtime.GOOS != "windows" {
		return fmt.Errorf("avto-installer hozircha faqat Windows uchun; %s da FFmpeg'ni qo'lda o'rnating", runtime.GOOS)
	}

	if progress == nil {
		progress = func(string, float64, float64, int, string) {}
	}

	// Har bir manbani ketma-ket sinab ko'ramiz
	var lastErr error
	for idx, m := range mirrors {
		progress("downloading", 0, 0, 0, fmt.Sprintf("[%d/%d] %s'dan yuklanmoqda...", idx+1, len(mirrors), m.Name))

		tmpZip := filepath.Join(os.TempDir(), fmt.Sprintf("cam2you-ffmpeg-%d.zip", time.Now().Unix()))

		err := download(ctx, m.URL, tmpZip, progress)
		if err != nil {
			os.Remove(tmpZip)
			lastErr = err
			progress("downloading", 0, 0, 0, fmt.Sprintf("Bu manbada xato: %v — keyingi manbaga o'tilmoqda", err))
			continue
		}

		// Chiqarish
		progress("extracting", 0, 0, 0, "FFmpeg arxivdan chiqarilmoqda...")
		err = extractBinaries(tmpZip, i.BinDir, progress)
		os.Remove(tmpZip)

		if err != nil {
			lastErr = err
			continue
		}

		// Tasdiq
		if i.IsInstalled() {
			progress("done", 100, 0, 0, "FFmpeg muvaffaqiyatli o'rnatildi")
			return nil
		}
		lastErr = errors.New("ffmpeg.exe arxivdan topilmadi")
	}

	return fmt.Errorf("barcha manbalardan yuklab bo'lmadi (oxirgi xato: %w)", lastErr)
}

// InstallFromFile — foydalanuvchi qo'lda yuklab olgan ffmpeg.exe yoki ZIP'dan o'rnatadi.
// path — ffmpeg.exe yoki .zip faylga to'liq yo'l bo'lishi mumkin.
func (i *Installer) InstallFromFile(srcPath string, progress ProgressFunc) error {
	if progress == nil {
		progress = func(string, float64, float64, int, string) {}
	}

	info, err := os.Stat(srcPath)
	if err != nil {
		return fmt.Errorf("fayl topilmadi: %w", err)
	}
	if info.IsDir() {
		return errors.New("fayl emas, papka tanlandi")
	}

	// .zip bo'lsa — chiqaramiz
	if strings.HasSuffix(strings.ToLower(srcPath), ".zip") {
		progress("extracting", 0, 0, 0, "ZIP'dan ffmpeg.exe va ffprobe.exe chiqarilmoqda...")
		if err := extractBinaries(srcPath, i.BinDir, progress); err != nil {
			return fmt.Errorf("chiqarish xatoligi: %w", err)
		}
	} else if strings.HasSuffix(strings.ToLower(srcPath), ".exe") {
		// ffmpeg.exe bo'lsa — uni va yonidagi ffprobe.exe ni nusxalaymiz
		progress("extracting", 0, 0, 0, "ffmpeg.exe nusxalanmoqda...")
		if err := copyFile(srcPath, i.FFmpegBin); err != nil {
			return fmt.Errorf("ffmpeg.exe nusxalanmadi: %w", err)
		}
		// ffprobe.exe ni shu papkada qidiramiz
		srcDir := filepath.Dir(srcPath)
		probeSrc := filepath.Join(srcDir, "ffprobe.exe")
		if fileExists(probeSrc) {
			if err := copyFile(probeSrc, i.FFprobeBin); err != nil {
				return fmt.Errorf("ffprobe.exe nusxalanmadi: %w", err)
			}
		}
	} else {
		return errors.New("noma'lum format — .exe yoki .zip kutilgan")
	}

	if !i.IsInstalled() {
		return errors.New("o'rnatildi, lekin tekshiruv muvaffaqiyatsiz")
	}
	progress("done", 100, 0, 0, "FFmpeg muvaffaqiyatli o'rnatildi")
	return nil
}

// copyFile — fayl nusxalash yordamchisi.
func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.OpenFile(dst, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o755)
	if err != nil {
		return err
	}
	defer out.Close()
	_, err = io.Copy(out, in)
	return err
}

// download — fayl yuklash + progress hisoblash.
func download(ctx context.Context, url, dst string, progress ProgressFunc) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return err
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("HTTP %d: %s", resp.StatusCode, resp.Status)
	}

	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer out.Close()

	total := resp.ContentLength
	pr := &progressReader{
		reader:   resp.Body,
		total:    total,
		callback: progress,
		stage:    "downloading",
		started:  time.Now(),
		lastTime: time.Now(),
	}

	_, err = io.Copy(out, pr)
	return err
}

// progressReader — io.Reader wrapper'i, har 256KB'da progress event yuboradi.
// Yuklash tezligi (MB/s) va qolgan vaqt (ETA) ham hisoblanadi.
type progressReader struct {
	reader     io.Reader
	total      int64
	read       int64
	lastReport int64
	lastBytes  int64
	lastTime   time.Time
	started    time.Time
	speedMBps  float64 // moving average
	callback   ProgressFunc
	stage      string
}

func (p *progressReader) Read(buf []byte) (int, error) {
	n, err := p.reader.Read(buf)
	p.read += int64(n)

	// Har 256KB yoki har 500ms'da yangilash
	now := time.Now()
	bytesSinceLastReport := p.read - p.lastReport
	timeSinceLastReport := now.Sub(p.lastTime)

	if bytesSinceLastReport > 256*1024 || timeSinceLastReport > 500*time.Millisecond {
		// Hozirgi tezlik (instant)
		var instantMBps float64
		if timeSinceLastReport > 0 {
			instantMBps = float64(p.read-p.lastBytes) / timeSinceLastReport.Seconds() / (1024 * 1024)
		}

		// Moving average — yumshoqroq son ko'rsatish uchun (70% old, 30% yangi)
		if p.speedMBps == 0 {
			p.speedMBps = instantMBps
		} else {
			p.speedMBps = p.speedMBps*0.7 + instantMBps*0.3
		}

		// ETA
		etaSec := 0
		if p.total > 0 && p.speedMBps > 0.01 {
			remainingMB := float64(p.total-p.read) / (1024 * 1024)
			etaSec = int(remainingMB / p.speedMBps)
		}

		percent := -1.0
		if p.total > 0 {
			percent = float64(p.read) / float64(p.total) * 100
		}
		msg := fmt.Sprintf("%.1f / %.1f MB", float64(p.read)/1024/1024, float64(p.total)/1024/1024)
		p.callback(p.stage, percent, p.speedMBps, etaSec, msg)

		p.lastReport = p.read
		p.lastBytes = p.read
		p.lastTime = now
	}
	return n, err
}

// extractBinaries — ZIP'dan faqat ffmpeg.exe va ffprobe.exe ni chiqaradi.
// Gyan.dev arxivi shunday tuzilishga ega:
//
//	ffmpeg-N.N-essentials_build/
//	  ├── bin/
//	  │   ├── ffmpeg.exe       ← bu kerak
//	  │   ├── ffprobe.exe      ← bu kerak
//	  │   └── ffplay.exe       ← bu kerakmas (skip)
//	  ├── doc/                 ← skip
//	  └── presets/             ← skip
func extractBinaries(zipPath, destDir string, progress ProgressFunc) error {
	r, err := zip.OpenReader(zipPath)
	if err != nil {
		return err
	}
	defer r.Close()

	wanted := map[string]bool{
		"ffmpeg.exe":  true,
		"ffprobe.exe": true,
	}

	extracted := 0
	for _, f := range r.File {
		base := filepath.Base(f.Name)
		if !wanted[base] {
			continue
		}
		// Faqat bin/ ichidagi fayllarni olamiz (LICENSE.txt va boshqalarni qoldirib)
		if !strings.Contains(f.Name, "/bin/") {
			continue
		}

		progress("extracting", float64(extracted)/float64(len(wanted))*100, 0, 0, "chiqarilmoqda: "+base)

		if err := extractFile(f, filepath.Join(destDir, base)); err != nil {
			return fmt.Errorf("%s chiqarib bo'lmadi: %w", base, err)
		}
		extracted++
	}

	if extracted < len(wanted) {
		return fmt.Errorf("arxivda faqat %d/%d binary topildi", extracted, len(wanted))
	}
	return nil
}

func extractFile(f *zip.File, dst string) error {
	rc, err := f.Open()
	if err != nil {
		return err
	}
	defer rc.Close()

	out, err := os.OpenFile(dst, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o755)
	if err != nil {
		return err
	}
	defer out.Close()

	_, err = io.Copy(out, rc)
	return err
}

func fileExists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && !info.IsDir()
}
