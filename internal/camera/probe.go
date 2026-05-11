package camera

import (
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"strconv"
	"time"
)

// ProbeResult — kamerani tekshirish natijasi.
type ProbeResult struct {
	OK         bool   `json:"ok"`
	Codec      string `json:"codec"`      // h264, hevc, ...
	Width      int    `json:"width"`
	Height     int    `json:"height"`
	FPS        string `json:"fps"`        // "25/1", "30000/1001" kabi
	HasAudio   bool   `json:"hasAudio"`
	AudioCodec string `json:"audioCodec"` // aac, pcm_mulaw, ...
	Error      string `json:"error,omitempty"`
}

// Prober — ffprobe orqali RTSP'ni tekshirish.
type Prober struct {
	FFprobeBin string // ffprobe.exe to'liq yo'li
	Timeout    time.Duration
}

// NewProber — yangi Prober yaratadi.
func NewProber(ffprobeBin string) *Prober {
	return &Prober{
		FFprobeBin: ffprobeBin,
		Timeout:    10 * time.Second,
	}
}

// Probe — RTSP URL'ni tekshiradi. Timeout ichida ulanmasa, ProbeResult.OK=false va Error qaytadi.
//
// ffprobe -hide_banner -loglevel error -rtsp_transport tcp \
//
//	-show_streams -print_format json -timeout 5000000 rtsp://...
func (p *Prober) Probe(ctx context.Context, rtspURL string) (*ProbeResult, error) {
	if p.FFprobeBin == "" {
		return &ProbeResult{Error: "ffprobe topilmadi"}, nil
	}

	ctx, cancel := context.WithTimeout(ctx, p.Timeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, p.FFprobeBin,
		"-hide_banner",
		"-loglevel", "error",
		"-rtsp_transport", "tcp",
		// FFmpeg 7+'da -stimeout olib tashlangan, -rw_timeout ishlatamiz
		"-rw_timeout", "5000000", // 5 sekund
		"-show_streams",
		"-print_format", "json",
		rtspURL,
	)
	hideConsoleWindow(cmd) // Windows'da CMD oynasi ochilmasin

	out, err := cmd.Output()
	if err != nil {
		// stderr'da nima yozilgan bo'lsa olamiz
		errMsg := err.Error()
		if exitErr, ok := err.(*exec.ExitError); ok && len(exitErr.Stderr) > 0 {
			errMsg = string(exitErr.Stderr)
		}
		return &ProbeResult{OK: false, Error: errMsg}, nil
	}

	var resp struct {
		Streams []struct {
			CodecType string `json:"codec_type"`
			CodecName string `json:"codec_name"`
			Width     int    `json:"width"`
			Height    int    `json:"height"`
			RFrameRate string `json:"r_frame_rate"`
		} `json:"streams"`
	}
	if err := json.Unmarshal(out, &resp); err != nil {
		return &ProbeResult{OK: false, Error: "ffprobe javobini o'qib bo'lmadi: " + err.Error()}, nil
	}

	result := &ProbeResult{OK: true}
	for _, s := range resp.Streams {
		switch s.CodecType {
		case "video":
			result.Codec = s.CodecName
			result.Width = s.Width
			result.Height = s.Height
			result.FPS = s.RFrameRate
		case "audio":
			result.HasAudio = true
			result.AudioCodec = s.CodecName
		}
	}

	if result.Codec == "" {
		result.OK = false
		result.Error = "video oqimi topilmadi"
	}
	return result, nil
}

// FPSFloat — "30000/1001" kabi r_frame_rate stringini float'ga aylantirib qaytaradi.
// Foydali yordamchi, hozircha service'da ishlatilmaydi.
func FPSFloat(rFrameRate string) float64 {
	for i := 0; i < len(rFrameRate); i++ {
		if rFrameRate[i] == '/' {
			num, err1 := strconv.ParseFloat(rFrameRate[:i], 64)
			den, err2 := strconv.ParseFloat(rFrameRate[i+1:], 64)
			if err1 != nil || err2 != nil || den == 0 {
				return 0
			}
			return num / den
		}
	}
	f, _ := strconv.ParseFloat(rFrameRate, 64)
	return f
}

// String — log'lar uchun qisqacha tushuntirish.
func (p *ProbeResult) String() string {
	if !p.OK {
		return fmt.Sprintf("ulanish xatoligi: %s", p.Error)
	}
	audio := "audio yo'q"
	if p.HasAudio {
		audio = "audio: " + p.AudioCodec
	}
	return fmt.Sprintf("%dx%d %s @ %s, %s", p.Width, p.Height, p.Codec, p.FPS, audio)
}
