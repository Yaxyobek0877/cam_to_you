// Package ffmpeg — bu fayl FFmpeg buyruq argumentlarini quradi.
//
// Stream tiplari:
//   - Single: bitta kamera → bitta RTMP chiqishi (oddiy -c copy)
//   - Grid:   2x1 / 2x2 / 3x3 — N kamerani xstack bilan birlashtirish
//   - PiP:    "Picture-in-picture" — asosiy kamera + burchakda kichik kamera
//
// Encoder tanlash:
//   - NVENC (NVIDIA GPU)     → -c:v h264_nvenc (eng tez, RTX 3060 da idealal)
//   - QuickSync (Intel GPU)  → -c:v h264_qsv
//   - CPU (libx264)          → eng portativ, lekin CPU yaxlitlaydi
//   - Copy                   → eng yengil — qayta kodlamaydi (faqat single uchun ishlaydi)
package ffmpeg

import (
	"fmt"
	"strconv"
	"strings"
)

// Encoder — video kodlash uchun ishlatilgan vositani belgilaydi.
type Encoder string

const (
	EncoderAuto      Encoder = "auto"        // mavjudligiga qarab eng tez variantni tanlaydi
	EncoderNVENC     Encoder = "h264_nvenc"  // NVIDIA GPU
	EncoderQuickSync Encoder = "h264_qsv"    // Intel GPU
	EncoderAMF       Encoder = "h264_amf"    // AMD GPU
	EncoderX264      Encoder = "libx264"     // CPU (GPL)
	EncoderOpenH264  Encoder = "libopenh264" // CPU (LGPL fallback)
	EncoderCopy      Encoder = "copy"        // qayta kodlamaslik (faqat single uchun)
)

// Layout — bir nechta kamerani bitta stream'da joylashtirish.
type Layout string

const (
	LayoutSingle Layout = "single" // 1 kamera
	Layout1x2    Layout = "1x2"    // 2 kamera yonma-yon (1080p+1080p → 3840x1080)
	Layout2x1    Layout = "2x1"    // 2 kamera ustma-ust (1080p×2 ustun → 1920x2160)
	Layout2x2    Layout = "2x2"    // 4 kamera kvadrat (2x2 grid)
	Layout3x3    Layout = "3x3"    // 9 kamera 3x3 grid
	LayoutPiP    Layout = "pip"    // Picture-in-Picture
)

// CamerasNeeded — berilgan layout uchun zarur kameralar soni.
func (l Layout) CamerasNeeded() int {
	switch l {
	case LayoutSingle:
		return 1
	case Layout1x2, Layout2x1, LayoutPiP:
		return 2
	case Layout2x2:
		return 4
	case Layout3x3:
		return 9
	}
	return 0
}

// CameraInput — bitta RTSP manba haqida ma'lumot.
type CameraInput struct {
	RTSPURL string // to'liq URL: rtsp://user:pass@ip:554/Streaming/Channels/101
	UseTCP  bool   // true bo'lsa -rtsp_transport tcp (Hikvision uchun deyarli har doim)
}

// StreamConfig — bitta yashash uchun barcha sozlamalar.
type StreamConfig struct {
	// Kirish (kamera(lar))
	Cameras []CameraInput
	Layout  Layout

	// Video parametrlari
	Encoder    Encoder // qaysi encoder bilan kodlash
	Width      int     // chiqish kengligi (masalan 1920)
	Height     int     // chiqish balandligi (masalan 1080)
	FPS        int     // 25, 30 — odatda
	BitrateKbps int    // 4500 — 1080p uchun YouTube tavsiyasi

	// Audio
	Audio AudioMode

	// Chiqish — RTMP destination
	RTMPURL string // rtmp://a.rtmp.youtube.com/live2/{STREAM_KEY}
}

// AudioMode — audio'ni qanday yo'naltirishni belgilaydi.
type AudioMode struct {
	// Mode: "first" — birinchi kameradan audio
	//       "muted" — audio yo'q
	//       "index" — Source indeksiga ko'ra kamera audio'si (CameraSource'da ko'rsatilgan)
	Mode         string
	CameraSource int // "index" mode'da qaysi kamera audio'si
}

// Build — StreamConfig'dan FFmpeg argumentlarini quradi.
// Birinchi qiymat — ffmpeg.exe ga beriladigan slice argumentlar (binary'ning o'zisiz).
//
// Misol natija (single, NVENC, copy):
//
//	-rtsp_transport tcp -i rtsp://... -c:v copy -c:a aac -b:a 128k -f flv rtmp://...
func Build(cfg StreamConfig) ([]string, error) {
	if cfg.RTMPURL == "" {
		return nil, fmt.Errorf("RTMP URL bo'sh")
	}
	needed := cfg.Layout.CamerasNeeded()
	if needed == 0 {
		return nil, fmt.Errorf("noto'g'ri layout: %s", cfg.Layout)
	}
	if len(cfg.Cameras) < needed {
		return nil, fmt.Errorf("%s layout %d kamera talab qiladi, %d berildi", cfg.Layout, needed, len(cfg.Cameras))
	}

	args := []string{}

	// 1) Global flaglar (loglarni soddalashtirish)
	args = append(args, "-hide_banner", "-loglevel", "warning")

	// 2) Har bir kamera uchun -i bilan kirish
	for i := 0; i < needed; i++ {
		cam := cfg.Cameras[i]
		if cam.UseTCP {
			args = append(args, "-rtsp_transport", "tcp")
		}
		// Qisqa zaif ulanishlar uchun timeout — yangi ffmpeg'da -stimeout o'rniga -rw_timeout
		// (eski versiyada -stimeout, FFmpeg 7+ versiyada olib tashlandi)
		args = append(args, "-rw_timeout", "10000000") // 10 sekund mikrosekundlarda
		args = append(args, "-i", cam.RTSPURL)
	}

	// 3) Layout'ga qarab filter va map argumentlari
	if cfg.Layout == LayoutSingle {
		args = append(args, buildSingleArgs(cfg)...)
	} else {
		args = append(args, buildCompositeArgs(cfg)...)
	}

	// 4) Audio
	args = append(args, buildAudioArgs(cfg)...)

	// 5) Chiqish
	args = append(args,
		"-f", "flv",
		"-flvflags", "no_duration_filesize", // YouTube uchun barqarorlik
		cfg.RTMPURL,
	)

	return args, nil
}

// buildSingleArgs — bitta kamera holatida video qismi.
func buildSingleArgs(cfg StreamConfig) []string {
	if cfg.Encoder == EncoderCopy {
		// Qayta kodlash yo'q — eng yengil
		return []string{"-c:v", "copy"}
	}
	return videoEncodeArgs(cfg)
}

// buildCompositeArgs — bir nechta kamera grid yoki PiP holatida.
// filter_complex orqali kameralarni birlashtirib, [v] degan nomli oqim yaratiladi.
func buildCompositeArgs(cfg StreamConfig) []string {
	filter := buildFilterComplex(cfg)
	args := []string{"-filter_complex", filter, "-map", "[v]"}
	args = append(args, videoEncodeArgs(cfg)...)
	return args
}

// buildFilterComplex — layout'ga qarab filter_complex string quradi.
// Har bir kameraning chiqishi cfg.Width/cfg.Height ga moslashtiriladi.
func buildFilterComplex(cfg StreamConfig) string {
	n := cfg.Layout.CamerasNeeded()

	// Avvalo har bir kameraning chiqishini standart o'lchamga keltiramiz.
	// PiP holatida burchakdagi kamera kichikroq bo'ladi — alohida shu yerda hisoblaymiz.
	switch cfg.Layout {
	case Layout1x2:
		// Yonma-yon: har biri (W/2)x H
		w, h := cfg.Width/2, cfg.Height
		return fmt.Sprintf(
			"[0:v]scale=%d:%d,setsar=1[v0];[1:v]scale=%d:%d,setsar=1[v1];[v0][v1]hstack=inputs=2[v]",
			w, h, w, h,
		)
	case Layout2x1:
		// Ustma-ust: har biri W x (H/2)
		w, h := cfg.Width, cfg.Height/2
		return fmt.Sprintf(
			"[0:v]scale=%d:%d,setsar=1[v0];[1:v]scale=%d:%d,setsar=1[v1];[v0][v1]vstack=inputs=2[v]",
			w, h, w, h,
		)
	case Layout2x2:
		// 2x2 grid: har biri (W/2) x (H/2)
		w, h := cfg.Width/2, cfg.Height/2
		var b strings.Builder
		for i := 0; i < 4; i++ {
			fmt.Fprintf(&b, "[%d:v]scale=%d:%d,setsar=1[v%d];", i, w, h, i)
		}
		// xstack layout: ikki ustun, ikki qator
		b.WriteString("[v0][v1][v2][v3]xstack=inputs=4:layout=0_0|w0_0|0_h0|w0_h0[v]")
		return b.String()
	case Layout3x3:
		// 3x3 grid: har biri (W/3) x (H/3)
		w, h := cfg.Width/3, cfg.Height/3
		var b strings.Builder
		for i := 0; i < 9; i++ {
			fmt.Fprintf(&b, "[%d:v]scale=%d:%d,setsar=1[v%d];", i, w, h, i)
		}
		// 3x3 layout pozitsiyalari
		layout := []string{
			"0_0", "w0_0", "w0+w1_0",
			"0_h0", "w0_h0", "w0+w1_h0",
			"0_h0+h1", "w0_h0+h1", "w0+w1_h0+h1",
		}
		b.WriteString("[v0][v1][v2][v3][v4][v5][v6][v7][v8]xstack=inputs=9:layout=")
		b.WriteString(strings.Join(layout, "|"))
		b.WriteString("[v]")
		return b.String()
	case LayoutPiP:
		// Asosiy: to'liq W x H. Burchakdagi: W/4 x H/4 o'ng pastki burchakda.
		pipW, pipH := cfg.Width/4, cfg.Height/4
		return fmt.Sprintf(
			"[0:v]scale=%d:%d,setsar=1[main];"+
				"[1:v]scale=%d:%d,setsar=1[pip];"+
				"[main][pip]overlay=W-w-20:H-h-20[v]",
			cfg.Width, cfg.Height, pipW, pipH,
		)
	}
	// Default — birinchi kamerani aynan o'tkazish
	_ = n
	return fmt.Sprintf("[0:v]scale=%d:%d,setsar=1[v]", cfg.Width, cfg.Height)
}

// videoEncodeArgs — encoder, bitrate, fps argumentlarini quradi.
func videoEncodeArgs(cfg StreamConfig) []string {
	enc := cfg.Encoder
	args := []string{"-c:v", string(enc)}

	switch enc {
	case EncoderNVENC:
		// NVIDIA NVENC — RTX 3060 da real-time osongina
		args = append(args,
			"-preset", "p4", // p1=eng tez, p7=eng sifatli; p4=balanced
			"-tune", "ll", // low latency (live uchun)
			"-rc", "cbr", // constant bitrate — YouTube xohlaydi
			"-b:v", strconv.Itoa(cfg.BitrateKbps)+"k",
			"-maxrate", strconv.Itoa(cfg.BitrateKbps)+"k",
			"-bufsize", strconv.Itoa(cfg.BitrateKbps*2)+"k",
			"-g", strconv.Itoa(cfg.FPS*2), // har 2 sekundda keyframe
		)
	case EncoderQuickSync:
		args = append(args,
			"-preset", "veryfast",
			"-b:v", strconv.Itoa(cfg.BitrateKbps)+"k",
			"-maxrate", strconv.Itoa(cfg.BitrateKbps)+"k",
			"-bufsize", strconv.Itoa(cfg.BitrateKbps*2)+"k",
			"-g", strconv.Itoa(cfg.FPS*2),
		)
	case EncoderX264:
		// CPU encoder — tezroq preset = kamroq sifat lekin kamroq CPU
		args = append(args,
			"-preset", "veryfast",
			"-tune", "zerolatency",
			"-b:v", strconv.Itoa(cfg.BitrateKbps)+"k",
			"-maxrate", strconv.Itoa(cfg.BitrateKbps)+"k",
			"-bufsize", strconv.Itoa(cfg.BitrateKbps*2)+"k",
			"-g", strconv.Itoa(cfg.FPS*2),
			"-pix_fmt", "yuv420p", // YouTube'ga mos
		)
	case EncoderOpenH264:
		// LGPL FFmpeg build'lar uchun fallback CPU encoder
		args = append(args,
			"-b:v", strconv.Itoa(cfg.BitrateKbps)+"k",
			"-g", strconv.Itoa(cfg.FPS*2),
			"-profile:v", "main",
			"-pix_fmt", "yuv420p",
		)
	case EncoderAMF:
		// AMD GPU
		args = append(args,
			"-quality", "speed",
			"-rc", "cbr",
			"-b:v", strconv.Itoa(cfg.BitrateKbps)+"k",
			"-maxrate", strconv.Itoa(cfg.BitrateKbps)+"k",
			"-bufsize", strconv.Itoa(cfg.BitrateKbps*2)+"k",
			"-g", strconv.Itoa(cfg.FPS*2),
			"-pix_fmt", "yuv420p",
		)
	}

	args = append(args, "-r", strconv.Itoa(cfg.FPS))
	return args
}

// buildAudioArgs — Audio map'ni quradi.
func buildAudioArgs(cfg StreamConfig) []string {
	switch cfg.Audio.Mode {
	case "muted":
		return []string{"-an"} // -an = audio yo'q
	case "index":
		idx := cfg.Audio.CameraSource
		if idx < 0 || idx >= len(cfg.Cameras) {
			idx = 0
		}
		return []string{
			"-map", fmt.Sprintf("%d:a?", idx), // ? — audio bo'lmasa skip
			"-c:a", "aac",
			"-b:a", "128k",
			"-ar", "44100",
		}
	default: // "first" yoki bo'sh
		return []string{
			"-map", "0:a?",
			"-c:a", "aac",
			"-b:a", "128k",
			"-ar", "44100",
		}
	}
}
