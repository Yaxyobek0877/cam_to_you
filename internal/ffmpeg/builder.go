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
	EncoderMF        Encoder = "h264_mf"     // Windows MediaFoundation (built-in, OBS/Twitch ham ishlatadi)
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

	// 1) Global flaglar
	// info darajada — connection holatini va mumkin bo'lgan xatolarni to'liq ko'rish uchun.
	// forwardLogs UI'ga faqat warning+ jo'natadi, lekin lastErr to'liq logni saqlaydi.
	args = append(args, "-hide_banner", "-loglevel", "info")

	// 2) Har bir kamera uchun -i bilan kirish
	// NVENC encoder bo'lsa, CUDA hardware decode'ni yoqamiz —
	// HEVC kameralar uchun ZARUR (CPU decode realtime'dan sekin va
	// RTSP buferi to'ladi, kamera ulanishni yopadi).
	useCudaDecode := cfg.Encoder == EncoderNVENC

	for i := 0; i < needed; i++ {
		cam := cfg.Cameras[i]

		// CUDA hardware decode — HEVC manba uchun katta tezlik (RTX 3060)
		if useCudaDecode {
			args = append(args, "-hwaccel", "cuda", "-hwaccel_output_format", "cuda")
		}

		if cam.UseTCP {
			args = append(args, "-rtsp_transport", "tcp")
		}

		// RTSP socket I/O timeout — yangi FFmpeg builds'da -stimeout o'rniga -timeout.
		// Cheksiz kutishni oldini oladi: agar kamera 10s ichida javob bermasa, FFmpeg
		// uziladi va auto-restart kiradi (yo'qotilgan sessiyaga abadiy yopishmaslik).
		//
		// MUHIM kuzatuv: -fflags +discardcorrupt va -reorder_queue_size 0 ni
		// QO'SHMASLIK kerak — HEVC decoder'da "Error constructing the frame RPS"
		// xatolarini keltirib chiqaradi (kerakli reference frame'larni tashlab yuboradi).
		// -nobuffer ham B-frame ko'p bo'lgan HEVC oqimlari uchun yomon.
		args = append(args,
			"-timeout", "10000000",
		)

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
	// -rtmp_live live  — bu live oqim ekanini bildiradi (VOD emas)
	// -rtmp_buffer    — RTMP buferi (ms), zaif tarmoq uchun katta qiymat
	// -shortest         — agar audio yoki video oxirgacha kelmasa, qisqaroqning oxiri bilan to'xtaydi
	//                     LIVE rejimda kerakmas — olib tashlangan
	// -flush_packets 1  — paketlarni darrov yuborish (kichikroq buferlash)
	args = append(args,
		"-flush_packets", "1",
		"-f", "flv",
		"-flvflags", "no_duration_filesize",
	)
	// Faqat RTMP/RTMPS uchun live flag
	if isRTMP(cfg.RTMPURL) {
		// Eslatma: -rtmp_live va -rtmp_buffer aslida INPUT flaglari (output emas)
		// Output uchun ular ishlamaydi. flvflags yetarli.
	}
	args = append(args, cfg.RTMPURL)

	return args, nil
}

// isRTMP — URL RTMP/RTMPS protokolida ekanini tekshiradi.
func isRTMP(url string) bool {
	return strings.HasPrefix(url, "rtmp://") || strings.HasPrefix(url, "rtmps://")
}

// buildSingleArgs — bitta kamera holatida video qismi.
//
// MUHIM: -map 0:v explicit qo'shilgan. Bu kerak, chunki buildAudioArgs
// "-map 0:a?" ishlatadi va FFmpeg har qanday -map ishlatilganda
// avtomatik stream tanlashni o'chiradi.
func buildSingleArgs(cfg StreamConfig) []string {
	args := []string{"-map", "0:v"}
	if cfg.Encoder == EncoderCopy {
		// Qayta kodlash yo'q — eng yengil (Sub-stream H.264 bilan IDEAL)
		args = append(args, "-c:v", "copy")
		return args
	}
	// CPU encoderlarda — sozlangan W:H ga scale qilamiz + yuv420p format.
	// Bu ZARUR: aks holda kamera sub-stream 640x360 chiqsa, output ham 640x360
	// bo'ladi (foydalanuvchi 1080p tanlagan bo'lsa ham). YouTube past sifat ko'rsatadi.
	//
	// `force_original_aspect_ratio=decrease` — agar manba aspect ratio farq qilsa,
	// chetga qora qora chiziqlar emas, balki passdek-fitiziya saqlab kichiklashtirish.
	// `pad` — qoldiq pikselni qora bilan to'ldirish (chetlari toza ko'rinadi).
	//
	// NVENC esa CUDA hardware path bilan ishlaydi — scale_cuda foydalanish kerak,
	// lekin hozircha NVENC'da scaling qilmaymiz (kamera sub-stream odatda kerakli
	// resolution'da bo'ladi). Yangilash kerak bo'lganda alohida fix.
	if cfg.Encoder != EncoderNVENC {
		// `scale=W:H:in_range=full:out_range=tv` — Hikvision yuvj420p (PC/full range)
		// → YouTube yuv420p (TV/limited range). Bu `swscaler ... deprecated pixel
		// format ... set range correctly` warning'ini yo'qotadi.
		//
		// h264_mf MediaFoundation encoder nv12 ni qabul qiladi; boshqalar yuv420p.
		pixFmt := "yuv420p"
		if cfg.Encoder == EncoderMF {
			pixFmt = "nv12"
		}
		filter := fmt.Sprintf(
			"scale=%d:%d:force_original_aspect_ratio=decrease:in_range=full:out_range=tv,"+
				"pad=%d:%d:(ow-iw)/2:(oh-ih)/2:color=black,"+
				"format=%s",
			cfg.Width, cfg.Height, cfg.Width, cfg.Height, pixFmt,
		)
		args = append(args, "-vf", filter)
	}
	args = append(args, videoEncodeArgs(cfg)...)
	return args
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

	// MUHIM: Hikvision yuvj420p (full range) yuboradi, YouTube yuv420p (TV range)
	// kutadi. -pix_fmt yuv420p barcha encoderlarda swscaler avtomatik konvertatsiyani
	// to'g'ri yo'naltiradi. Bu warning'ni ham yo'qotadi.

	switch enc {
	case EncoderNVENC:
		// NVIDIA NVENC — RTX 3060 da real-time osongina.
		// p1 (eng tez) preset — HEVC→H.264 transcode uchun zarur, p4 sekinroq.
		// CUDA hardware decode (input args'da yoqilgan) bilan ishlatiladi.
		// -pix_fmt yuv420p OLIB TASHLANDI — CUDA path bilan mos kelmaydi.
		// Output yuvj420p bo'ladi, YouTube qabul qiladi.
		gop := strconv.Itoa(cfg.FPS * 2)
		args = append(args,
			"-preset", "p1", // p1=eng tez (live uchun zarur)
			"-tune", "ll", // low latency
			"-profile:v", "high", // YouTube xohlaydi
			"-rc", "cbr", // constant bitrate
			"-b:v", strconv.Itoa(cfg.BitrateKbps)+"k",
			"-maxrate", strconv.Itoa(cfg.BitrateKbps)+"k",
			"-bufsize", strconv.Itoa(cfg.BitrateKbps*2)+"k",
			"-g", gop, // har 2 sek keyframe (YouTube majburiy)
			"-keyint_min", gop, // qat'iy interval
			"-bf", "0", // B-frames yo'q
		)
	case EncoderQuickSync:
		gop := strconv.Itoa(cfg.FPS * 2)
		args = append(args,
			"-preset", "veryfast",
			"-profile:v", "high",
			"-b:v", strconv.Itoa(cfg.BitrateKbps)+"k",
			"-maxrate", strconv.Itoa(cfg.BitrateKbps)+"k",
			"-bufsize", strconv.Itoa(cfg.BitrateKbps*2)+"k",
			"-g", gop,
			"-keyint_min", gop,
			"-bf", "0",
			"-pix_fmt", "yuv420p",
		)
	case EncoderX264:
		// CPU encoder — tezroq preset = kamroq sifat lekin kamroq CPU
		gop := strconv.Itoa(cfg.FPS * 2)
		args = append(args,
			"-preset", "veryfast",
			"-tune", "zerolatency",
			"-profile:v", "high",
			"-b:v", strconv.Itoa(cfg.BitrateKbps)+"k",
			"-maxrate", strconv.Itoa(cfg.BitrateKbps)+"k",
			"-bufsize", strconv.Itoa(cfg.BitrateKbps*2)+"k",
			"-g", gop,
			"-keyint_min", gop,
			"-sc_threshold", "0", // scene-cut keyframes o'chiriladi
			"-pix_fmt", "yuv420p", // YouTube'ga mos
		)
	case EncoderOpenH264:
		// LGPL FFmpeg build'lar uchun fallback CPU encoder (libx264 yo'q paytda).
		//
		// MUHIM tarix:
		//   v0.1.20 da `-profile:v main` ishlatilgandi — bu libopenh264 da
		//   "layerId(0) doesn't support profile(578), change to UNSPECIFIC profile"
		//   warning'ini keltirib chiqarar edi. UNSPECIFIC profile = profile_idc=0
		//   H.264 oqimi → YouTube qabul qiladi, lekin DEKOD QILA OLMAYDI.
		//   Natija: Cam2You "stream ishlamoqda" deydi, YouTube "translatsiya
		//   kelmayapti" deydi.
		//
		//   v0.1.21 da `-profile:v high` ga o'tdik — bu PRO_HIGH=100 ga aniq
		//   xaritalanadi va YouTube to'g'ri dekod qiladi. OBS va boshqa
		//   software ham high profile ishlatadi.
		//
		// MUHIM bayroqlar:
		//   - profile:v high      — UNSPECIFIC profile bug'ini chetlab o'tadi
		//   - rc_mode bitrate     — default "quality" emas, target bitrate'ni hurmat qilsin
		//   - allow_skip_frames 1 — bitrate cheklovini qondirish uchun frame skip ruxsat
		//   - keyint_min          — qat'iy keyframe oralig'i (YouTube majburiy)
		//
		// `+global_header` va `dump_extra` v0.1.21 da OLIB TASHLANDI — FLV muxer
		// default holatda SPS/PPS'ni to'g'ri joylashtiradi, bu bayroqlar keraksiz
		// va ba'zi YouTube ingest server'larida muammo qiladi.
		gop := strconv.Itoa(cfg.FPS * 2)
		args = append(args,
			"-profile:v", "high",
			"-rc_mode", "bitrate",
			"-allow_skip_frames", "1",
			"-b:v", strconv.Itoa(cfg.BitrateKbps)+"k",
			"-g", gop,
			"-keyint_min", gop,
			"-pix_fmt", "yuv420p",
		)
	case EncoderAMF:
		// AMD GPU
		gop := strconv.Itoa(cfg.FPS * 2)
		args = append(args,
			"-quality", "speed",
			"-profile:v", "high",
			"-rc", "cbr",
			"-b:v", strconv.Itoa(cfg.BitrateKbps)+"k",
			"-maxrate", strconv.Itoa(cfg.BitrateKbps)+"k",
			"-bufsize", strconv.Itoa(cfg.BitrateKbps*2)+"k",
			"-g", gop,
			"-bf", "0",
			"-pix_fmt", "yuv420p",
		)
	case EncoderMF:
		// Windows MediaFoundation — Microsoft'ning o'rnatilgan H.264 encoder'i.
		// OBS, vMix va boshqa software ham shu encoder ishlatadi. libopenh264 ga
		// nisbatan AFZAL: profile aniq xaritalanadi va YouTube to'liq qabul qiladi.
		//
		// MUHIM: h264_mf nv12 pixel format'ni kutadi (yuv420p emas).
		// Bu buildSingleArgs'da scale filter chiqishini h264_mf uchun nv12 ga
		// keltirilishi kerak — quyida `pix_fmt nv12` shu uchun.
		gop := strconv.Itoa(cfg.FPS * 2)
		args = append(args,
			"-rate_control", "cbr",
			"-b:v", strconv.Itoa(cfg.BitrateKbps)+"k",
			"-g", gop,
			"-bf", "0",
			"-pix_fmt", "nv12",
		)
	}

	// -r OLIB TASHLANDI: kamera fps'ini source'dan o'tkazamiz.
	// Aks holda Hikvision 25fps source 30fps'ga majburlanib, frame duplication
	// va PTS muammolarini chiqaradi (kamera RTSP ulanishni yopib qo'yadi).
	// User'ning quality.FPS GOP hisoblash uchun saqlanadi (g, keyint_min).
	return args
}

// buildAudioArgs — Audio map'ni quradi.
//
// YouTube talablari:
//   - AAC-LC codec
//   - Stereo (2 kanal) — ko'p kameralar mono yuboradi, biz upmix qilamiz
//   - 44.1 yoki 48 kHz
//   - 128-160 kbps
//
// Hikvision PCM mu-law 8kHz mono → AAC 44.1kHz stereo 128k.
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
			"-map", fmt.Sprintf("%d:a?", idx),
			"-c:a", "aac",
			"-b:a", "128k",
			"-ar", "44100",
			"-ac", "2", // YouTube uchun stereo majburiy
		}
	default: // "first" yoki bo'sh
		return []string{
			"-map", "0:a?",
			"-c:a", "aac",
			"-b:a", "128k",
			"-ar", "44100",
			"-ac", "2", // mono → stereo upmix
		}
	}
}
