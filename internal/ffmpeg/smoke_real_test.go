// Package ffmpeg — smoke_real_test.go: real DB konfiguratsiyasi bilan
// FFmpeg buyrug'ini tuzib, ishga tushirib, natijani tekshiradi.
// Faqat -tags smoke bilan ishlaydi (default test'larga ta'sir qilmaydi).
//go:build smoke

package ffmpeg_test

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"

	"cam_to_you/internal/ffmpeg"
	"cam_to_you/internal/models"

	_ "modernc.org/sqlite"
)

func TestRealStreamCommand(t *testing.T) {
	dbPath := os.Getenv("CAM2YOU_DB")
	if dbPath == "" {
		dbPath = os.Getenv("APPDATA") + `\Cam2You\app.db`
	}
	ffmpegBin := os.Getenv("CAM2YOU_FFMPEG")
	if ffmpegBin == "" {
		ffmpegBin = os.Getenv("APPDATA") + `\Cam2You\bin\ffmpeg.exe`
	}

	db, err := sql.Open("sqlite", dbPath)
	if err != nil { t.Fatalf("DB open: %v", err) }
	defer db.Close()

	st := &models.Stream{}
	var camIDsJSON, layoutStr, qualityStr, encStr, platformStr string
	var autoRestart int
	var createdAt, updatedAt string
	err = db.QueryRow(`SELECT id, name, layout, camera_ids, quality, encoder,
		audio_mode, audio_camera_index, platform, stream_key, custom_url,
		auto_restart, max_restarts, restart_delay_ms, created_at, updated_at
		FROM streams ORDER BY id LIMIT 1`).Scan(
		&st.ID, &st.Name, &layoutStr, &camIDsJSON, &qualityStr, &encStr,
		&st.AudioMode, &st.AudioCameraIndex, &platformStr, &st.StreamKey, &st.CustomURL,
		&autoRestart, &st.MaxRestarts, &st.RestartDelayMs, &createdAt, &updatedAt)
	if err != nil { t.Fatalf("stream: %v", err) }
	st.Layout = ffmpeg.Layout(layoutStr)
	st.Quality = models.Quality(qualityStr)
	st.Encoder = ffmpeg.Encoder(encStr)
	st.Platform = models.Platform(platformStr)
	st.AutoRestart = autoRestart == 1
	_ = json.Unmarshal([]byte(camIDsJSON), &st.CameraIDs)

	t.Logf("STREAM: name=%q layout=%s encoder=%s platform=%s key=%s",
		st.Name, st.Layout, st.Encoder, st.Platform, mask(st.StreamKey))

	c := &models.Camera{}
	var useSub int
	var camCreated, camUpdated string
	err = db.QueryRow(`SELECT id, name, vendor, host, port, username, password,
		channel, use_sub_stream, raw_rtsp_url, created_at, updated_at FROM cameras ORDER BY id LIMIT 1`).Scan(
		&c.ID, &c.Name, &c.Vendor, &c.Host, &c.Port, &c.Username, &c.Password,
		&c.Channel, &useSub, &c.RawRTSPURL, &camCreated, &camUpdated)
	if err != nil { t.Fatalf("camera: %v", err) }
	c.UseSubStream = useSub == 1
	t.Logf("CAMERA: vendor=%s host=%s ch=%d useSub=%v",
		c.Vendor, c.Host, c.Channel, c.UseSubStream)

	cfg := ffmpeg.StreamConfig{
		Layout:  st.Layout,
		Encoder: st.Encoder,
		Audio:   ffmpeg.AudioMode{Mode: st.AudioMode, CameraSource: st.AudioCameraIndex},
		RTMPURL: st.FullRTMPURL(),
	}
	cfg.Width, cfg.Height, cfg.FPS, cfg.BitrateKbps = st.Quality.Resolve()
	cfg.Cameras = []ffmpeg.CameraInput{{RTSPURL: c.BuildRTSPURL(), UseTCP: true}}

	encs := ffmpeg.DetectEncoders(ffmpegBin)
	resolved, err := ffmpeg.ResolveEncoder(cfg.Encoder, encs)
	if err != nil { t.Fatalf("resolve enc: %v", err) }
	cfg.Encoder = resolved
	t.Logf("ENCODER (resolved): %s", resolved)

	args, err := ffmpeg.Build(cfg)
	if err != nil { t.Fatalf("Build: %v", err) }

	fmt.Println("=== FFMPEG ARGUMENTLARI ===")
	for _, a := range args {
		fmt.Printf("  %s\n", mask(a))
	}

	// -t 20 qo'shamiz cheklash uchun (URL'dan oldin)
	finalArgs := append([]string{}, args...)
	url := finalArgs[len(finalArgs)-1]
	finalArgs = append(finalArgs[:len(finalArgs)-1], "-t", "20", url)

	ctx, cancel := context.WithTimeout(context.Background(), 35*time.Second)
	defer cancel()

	fmt.Println("\n=== ISHGA TUSHIRISH (20 sek) ===")
	cmd := exec.CommandContext(ctx, ffmpegBin, finalArgs...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	start := time.Now()
	err = cmd.Run()
	elapsed := time.Since(start)

	fmt.Printf("\n=== NATIJA ===\nDavom: %.1f sek\n", elapsed.Seconds())
	if err != nil {
		t.Errorf("FFmpeg xato: %v (exit=%d)", err, cmd.ProcessState.ExitCode())
		return
	}
	if elapsed < 15*time.Second {
		t.Errorf("⚠️ TEZ CHIQDI (%v) — kamera sessiya cheklovi", elapsed)
		return
	}
	t.Logf("✅ 15+ sek davom etdi — barqaror")
}

func mask(s string) string {
	if strings.HasPrefix(s, "rtmp://") || strings.HasPrefix(s, "rtmps://") {
		idx := strings.LastIndex(s, "/")
		if idx > 0 && idx < len(s)-4 {
			return s[:idx+1] + s[idx+1:idx+5] + "***"
		}
	}
	if strings.Contains(s, "@") && strings.Contains(s, "://") {
		idx := strings.Index(s, "://")
		atIdx := strings.Index(s, "@")
		if idx >= 0 && atIdx > idx {
			return s[:idx+3] + "***:***" + s[atIdx:]
		}
	}
	return s
}
