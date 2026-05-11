package ffmpeg

import (
	"context"
	"os/exec"
	"strings"
	"sync"
	"time"
)

// DetectedEncoders — tizimda mavjud H.264 encoderlar.
type DetectedEncoders struct {
	NVENC       bool // NVIDIA GPU
	QuickSync   bool // Intel GPU
	AMF         bool // AMD GPU
	X264        bool // libx264 (GPL CPU encoder, eng sifatli)
	OpenH264    bool // libopenh264 (LGPL CPU encoder, fallback)
	MediaFound  bool // Windows MediaFoundation (h264_mf)
}

// BestH264 — uskunaga qarab eng tez H.264 encoder'ini qaytaradi.
// Tartib (v0.1.21 dan):
//
//	NVENC > QSV > AMF > X264 > MediaFoundation > OpenH264
//
// MUHIM o'zgartirish: MediaFoundation (h264_mf) endi OpenH264'dan AFZALROQ.
// Sabab: libopenh264 H.264 profile (main/high) ni "UNSPECIFIC" ga
// xaritalashga moyil — natijada YouTube qabul qiladi, lekin DEKOD QILA OLMAYDI.
// h264_mf Windows-native, OBS va boshqa professional software ham shu encoder
// ishlatadi. YouTube to'liq qabul qiladi.
//
// Foydalanuvchi qo'lda libopenh264'ni tanlasa ham ishlaydi, lekin "auto"da
// h264_mf afzal — kamroq surprise.
func (e DetectedEncoders) BestH264() string {
	switch {
	case e.NVENC:
		return "h264_nvenc"
	case e.QuickSync:
		return "h264_qsv"
	case e.AMF:
		return "h264_amf"
	case e.X264:
		return "libx264"
	case e.MediaFound:
		return "h264_mf"
	case e.OpenH264:
		return "libopenh264"
	}
	return ""
}

// HasEncoder — berilgan encoder nomi tizimda mavjudmi?
// Yo'q bo'lsa, encoder fallback'ga o'tish kerak.
func (e DetectedEncoders) HasEncoder(name string) bool {
	switch name {
	case "h264_nvenc", "hevc_nvenc":
		return e.NVENC
	case "h264_qsv", "hevc_qsv":
		return e.QuickSync
	case "h264_amf", "hevc_amf":
		return e.AMF
	case "libx264":
		return e.X264
	case "libopenh264":
		return e.OpenH264
	case "h264_mf":
		return e.MediaFound
	case "copy":
		return true // har doim mavjud
	}
	return false
}

var (
	detectedEncoders DetectedEncoders
	detectOnce       sync.Once
)

// DetectEncoders — ffmpeg.exe'dan encoder ro'yxatini olib, mavjudlarini aniqlaydi.
// Faqat birinchi chaqirig'da haqiqiy chaqirilad, qolganlari cache'dan keladi.
//
// Agar binary o'zgargan bo'lsa, ResetEncoderCache() chaqiring (FFmpeg qayta o'rnatilganda).
func DetectEncoders(ffmpegBin string) DetectedEncoders {
	detectOnce.Do(func() {
		detectedEncoders = detect(ffmpegBin)
	})
	return detectedEncoders
}

// ResetEncoderCache — keshni tozalaydi (FFmpeg auto-installer'dan keyin chaqirildi).
func ResetEncoderCache() {
	detectOnce = sync.Once{}
}

func detect(ffmpegBin string) DetectedEncoders {
	var d DetectedEncoders
	if ffmpegBin == "" {
		return d
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, ffmpegBin, "-hide_banner", "-encoders")
	hideConsoleWindow(cmd) // CMD oynasi ochilmasin
	out, err := cmd.Output()
	if err != nil {
		return d
	}

	text := string(out)
	d.NVENC = strings.Contains(text, "h264_nvenc")
	d.QuickSync = strings.Contains(text, "h264_qsv")
	d.AMF = strings.Contains(text, "h264_amf")
	d.X264 = strings.Contains(text, "libx264")
	d.OpenH264 = strings.Contains(text, "libopenh264")
	d.MediaFound = strings.Contains(text, "h264_mf")
	return d
}

// ResolveEncoder — foydalanuvchi tanlovi (yoki "auto") asosida haqiqiy encoder nomini qaytaradi.
//
// Misol:
//   - User "auto"  + RTX 3060 bor  → "h264_nvenc"
//   - User "x264"  + LGPL build    → "libopenh264" (fallback)
//   - User "copy"                  → "copy"
//   - User "nvenc" + GPU yo'q      → "" (xato — foydalanuvchini ogohlantiramiz)
func ResolveEncoder(userChoice Encoder, detected DetectedEncoders) (Encoder, error) {
	if userChoice == EncoderCopy {
		return EncoderCopy, nil
	}

	if userChoice == EncoderAuto {
		best := detected.BestH264()
		if best == "" {
			return "", encoderError("hech qanday H.264 encoder topilmadi")
		}
		return Encoder(best), nil
	}

	// Aniq tanlangan — mavjudmi tekshiramiz
	if detected.HasEncoder(string(userChoice)) {
		return userChoice, nil
	}

	// Fallback'ga o'tamiz
	switch userChoice {
	case EncoderX264:
		// X264 yo'q bo'lsa, openh264 yoki nvenc'ga o'tamiz
		if detected.OpenH264 {
			return Encoder("libopenh264"), nil
		}
		if best := detected.BestH264(); best != "" {
			return Encoder(best), nil
		}
	case EncoderNVENC:
		// NVIDIA yo'q — boshqa GPU yoki CPU'ga o'tamiz
		if best := detected.BestH264(); best != "" {
			return Encoder(best), nil
		}
	case EncoderQuickSync:
		// Intel yo'q — boshqa GPU yoki CPU
		if best := detected.BestH264(); best != "" {
			return Encoder(best), nil
		}
	}

	return "", encoderError("tanlangan encoder mavjud emas va fallback topilmadi")
}

type encoderErr string

func (e encoderErr) Error() string { return string(e) }
func encoderError(msg string) error { return encoderErr(msg) }
