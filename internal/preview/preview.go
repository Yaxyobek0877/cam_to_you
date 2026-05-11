// Package preview — kameralarni jonli ko'rish uchun RTSP→HLS bridge.
//
// Foydalanuvchi "Jonli ko'rish" tugmasini bosganda:
//  1. Start(cameraID, rtspURL) chaqiriladi
//  2. FFmpeg ishga tushadi, RTSP'ni HLS segmentlariga aylantirib %APPDATA%\Cam2You\previews\{id}\ ga yozadi
//  3. Frontend hls.js orqali /preview/{id}/index.m3u8 ni ijro etadi (AssetsHandler beradi)
//  4. Modal yopilganda Stop(cameraID) chaqiriladi
//
// Tezkor diqqat: HLS taxminan 3-5 sek latentlik beradi. Sub-second emas,
// lekin "kamera ishlayaptimi" tekshirish uchun yetarli.
package preview

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"cam_to_you/internal/ffmpeg"
)

// EventType — preview hodisalari turlari.
type EventType string

const (
	EventStarting   EventType = "starting"   // preview ishga tushyapti
	EventReady      EventType = "ready"      // birinchi segment yozildi, video tayyor
	EventLog        EventType = "log"        // FFmpeg log qatori
	EventError      EventType = "error"      // xatolik
	EventStopped    EventType = "stopped"    // to'xtatildi
)

// Event — preview servisidan UI'ga uzatiladigan hodisalar.
type Event struct {
	Type     EventType   `json:"type"`
	CameraID int64       `json:"cameraId"`
	Payload  interface{} `json:"payload,omitempty"`
}

// Service — aktiv preview'larni boshqaradi.
type Service struct {
	ffmpegBin  string // ffmpeg.exe yo'li
	previewDir string // %APPDATA%\Cam2You\previews

	mu          sync.RWMutex
	active      map[int64]*session
	subscribers []chan Event
}

// session — bitta aktiv preview.
type session struct {
	cameraID  int64
	runner    *ffmpeg.Runner
	dir       string
	startedAt time.Time
	cancel    context.CancelFunc
}

// New — yangi service yaratadi.
func New(ffmpegBin, previewDir string) *Service {
	return &Service{
		ffmpegBin:  ffmpegBin,
		previewDir: previewDir,
		active:     make(map[int64]*session),
	}
}

// SetFFmpegBin — FFmpeg auto-installer'dan keyin yangilash uchun.
func (s *Service) SetFFmpegBin(bin string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.ffmpegBin = bin
}

// Subscribe — preview hodisalarini berilgan kanalga yuboradi.
// Unsubscribe — qaytarilgan funksiyani chaqiring.
func (s *Service) Subscribe(ch chan Event) (unsubscribe func()) {
	s.mu.Lock()
	s.subscribers = append(s.subscribers, ch)
	s.mu.Unlock()
	return func() {
		s.mu.Lock()
		defer s.mu.Unlock()
		for i, c := range s.subscribers {
			if c == ch {
				s.subscribers = append(s.subscribers[:i], s.subscribers[i+1:]...)
				return
			}
		}
	}
}

// emit — hodisani barcha subscriber'larga yuboradi (non-blocking).
func (s *Service) emit(e Event) {
	s.mu.RLock()
	subs := make([]chan Event, len(s.subscribers))
	copy(subs, s.subscribers)
	s.mu.RUnlock()
	for _, ch := range subs {
		select {
		case ch <- e:
		default:
		}
	}
}

// Start — kamera uchun preview boshqalaydi. Allaqachon ishlayotgan bo'lsa, hech narsa qilmaydi.
func (s *Service) Start(cameraID int64, rtspURL string) error {
	if s.ffmpegBin == "" {
		return errors.New("FFmpeg topilmadi — avval Settings'da o'rnating")
	}
	if rtspURL == "" {
		return errors.New("RTSP URL bo'sh")
	}

	s.mu.Lock()
	if _, exists := s.active[cameraID]; exists {
		s.mu.Unlock()
		return nil // allaqachon ishlamoqda
	}

	camDir := filepath.Join(s.previewDir, fmt.Sprintf("%d", cameraID))
	// Eski fayllar bo'lsa tozalaymiz
	_ = os.RemoveAll(camDir)
	if err := os.MkdirAll(camDir, 0o755); err != nil {
		s.mu.Unlock()
		return fmt.Errorf("preview papkasini yarata olmadim: %w", err)
	}

	indexFile := filepath.Join(camDir, "index.m3u8")
	segPattern := filepath.Join(camDir, "seg_%d.ts")

	// Encoder aniqlanadi — har xil FFmpeg buildlar har xil encoderlarga ega.
	// Hikvision odatda HEVC (H.265) yuboradi, WebView2 hls.js HEVC'ni
	// o'qiy olmaydi — har doim H.264'ga transcode qilamiz.
	encs := ffmpeg.DetectEncoders(s.ffmpegBin)
	enc := encs.BestH264()
	if enc == "" {
		s.mu.Unlock()
		return errors.New("hech qanday H.264 encoder topilmadi (FFmpeg buildni qayta o'rnating)")
	}

	// Rezolyutsiya — encoder turiga qarab adaptiv
	// GPU encoderlar 720p osongina ko'taradi
	// CPU encoderlar uchun 480p — eski/zaif PC'larda ham silliq ishlaydi
	isGPU := enc == "h264_nvenc" || enc == "h264_qsv" || enc == "h264_amf"
	scale := "scale=-2:480"
	if isGPU {
		scale = "scale=-2:720"
	}

	args := []string{
		"-hide_banner",
		"-loglevel", "info", // info darajada — connection holatini ko'rish uchun
	}

	// Hardware decode — har qanday GPU (NVIDIA/AMD/Intel iGPU) bilan ishlaydi.
	// HEVC decode'ni GPU'da bajarib, CPU'ni faqat encode uchun bo'shatadi.
	// Yo'q bo'lsa, FFmpeg avtomatik software decode'ga qaytadi.
	args = append(args, "-hwaccel", "auto")

	args = append(args,
		"-rtsp_transport", "tcp",
		"-i", rtspURL,
		"-c:v", enc,
	)
	args = append(args, previewEncoderArgs(enc)...)
	args = append(args,
		"-vf", scale,
		"-an", // audio o'chirilgan
		"-pix_fmt", "yuv420p",
		"-f", "hls",
		"-hls_time", "2",
		"-hls_list_size", "4",
		"-hls_flags", "delete_segments+omit_endlist+independent_segments",
		"-hls_segment_type", "mpegts",
		"-hls_segment_filename", segPattern,
		indexFile,
	)

	ctx, cancel := context.WithCancel(context.Background())
	runner := ffmpeg.NewRunner(fmt.Sprintf("preview-%d", cameraID), s.ffmpegBin, args)

	sess := &session{
		cameraID:  cameraID,
		runner:    runner,
		dir:       camDir,
		startedAt: time.Now(),
		cancel:    cancel,
	}
	s.active[cameraID] = sess
	s.mu.Unlock()

	s.emit(Event{Type: EventStarting, CameraID: cameraID, Payload: map[string]interface{}{
		"encoder": enc,
		"rtspUrl": maskRTSPCredentials(rtspURL),
	}})

	if err := runner.Start(ctx); err != nil {
		s.mu.Lock()
		delete(s.active, cameraID)
		s.mu.Unlock()
		cancel()
		s.emit(Event{Type: EventError, CameraID: cameraID, Payload: err.Error()})
		return fmt.Errorf("ffmpeg ishga tushmadi: %w", err)
	}

	// FFmpeg log'larini hodisa sifatida uzatamiz
	logCh := make(chan ffmpeg.LogLine, 100)
	unsubLogs := runner.Subscribe(logCh)
	go s.forwardLogs(cameraID, logCh)

	// Birinchi segment yaratilganini kuzatamiz — bu UI'ga "Ready" signali bo'ladi
	go s.watchReady(cameraID, camDir)

	// Process chiqsa — xaritadan o'chiramiz va hodisani emit qilamiz
	go func() {
		<-runner.Done()
		unsubLogs()
		close(logCh)

		state := runner.State()
		errMsg := runner.LastError()

		s.mu.Lock()
		delete(s.active, cameraID)
		s.mu.Unlock()

		if state == ffmpeg.StateError {
			s.emit(Event{Type: EventError, CameraID: cameraID, Payload: errMsg})
		} else {
			s.emit(Event{Type: EventStopped, CameraID: cameraID})
		}
	}()

	return nil
}

// watchReady — index.m3u8 va birinchi segment yozilganini kutadi,
// keyin EventReady ni emit qiladi (UI "playing"ga o'tadi).
func (s *Service) watchReady(cameraID int64, camDir string) {
	indexPath := filepath.Join(camDir, "index.m3u8")
	deadline := time.Now().Add(30 * time.Second)

	for time.Now().Before(deadline) {
		// preview to'xtatilganmi?
		if !s.IsActive(cameraID) {
			return
		}
		info, err := os.Stat(indexPath)
		if err == nil && info.Size() > 50 {
			// Birinchi segment yozilgan
			s.emit(Event{Type: EventReady, CameraID: cameraID})
			return
		}
		time.Sleep(200 * time.Millisecond)
	}
	// 30s ichida tayyor bo'lmadi — xato emit qilamiz, lekin runner ishlasa to'xtatmaymiz
	s.emit(Event{Type: EventError, CameraID: cameraID, Payload: "30 sekund ichida video oqimi tayyor bo'lmadi"})
}

// forwardLogs — FFmpeg loglarini preview hodisalariga aylantiradi.
// Preview uchun HAMMA log darajalarini yuboramiz — foydalanuvchi nima bo'layotganini ko'rishi kerak.
func (s *Service) forwardLogs(cameraID int64, logCh <-chan ffmpeg.LogLine) {
	for ll := range logCh {
		s.emit(Event{
			Type:     EventLog,
			CameraID: cameraID,
			Payload: map[string]interface{}{
				"time":    ll.Time.Format(time.RFC3339),
				"level":   string(ll.Level),
				"message": ll.Message,
			},
		})
	}
}

// maskRTSPCredentials — RTSP URL'dagi user:pass'ni "***"ga almashtiradi (log uchun xavfsizroq).
func maskRTSPCredentials(url string) string {
	// rtsp://user:pass@host:port/path → rtsp://***@host:port/path
	atIdx := strings.LastIndex(url, "@")
	if atIdx == -1 {
		return url
	}
	schemeEnd := strings.Index(url, "://")
	if schemeEnd == -1 || schemeEnd >= atIdx {
		return url
	}
	return url[:schemeEnd+3] + "***@" + url[atIdx+1:]
}

// Stop — preview'ni to'xtatadi va fayllarni tozalaydi.
func (s *Service) Stop(cameraID int64) error {
	s.mu.Lock()
	sess, ok := s.active[cameraID]
	if !ok {
		s.mu.Unlock()
		return nil // ishlamayapti — bekor qilmaymiz, OK
	}
	delete(s.active, cameraID)
	s.mu.Unlock()

	sess.cancel()
	_ = sess.runner.Stop(3 * time.Second)

	// Fayllarni tozalash (kechikishimiz mumkin — UI bunga e'tibor bermaydi)
	go func() {
		time.Sleep(500 * time.Millisecond)
		_ = os.RemoveAll(sess.dir)
	}()
	s.emit(Event{Type: EventStopped, CameraID: cameraID})
	return nil
}

// StopAll — barcha preview'larni to'xtatadi (Shutdown uchun).
func (s *Service) StopAll() {
	s.mu.RLock()
	ids := make([]int64, 0, len(s.active))
	for id := range s.active {
		ids = append(ids, id)
	}
	s.mu.RUnlock()
	for _, id := range ids {
		_ = s.Stop(id)
	}
}

// IsActive — kamera preview'i ishlayaptimi?
func (s *Service) IsActive(cameraID int64) bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	_, ok := s.active[cameraID]
	return ok
}

// previewEncoderArgs — encoder turiga qarab tegishli sozlamalar.
// Preview uchun maksimal tezlik, minimal sifat (720p yetarli).
func previewEncoderArgs(encoder string) []string {
	switch encoder {
	case "h264_nvenc":
		return []string{
			"-preset", "p1", // p1=eng tez (NVENC)
			"-tune", "ull", // ultra low latency
			"-rc", "cbr",
			"-b:v", "1500k",
			"-maxrate", "1800k",
			"-bufsize", "3000k",
			"-g", "50",
			"-profile:v", "baseline",
		}
	case "h264_qsv":
		return []string{
			"-preset", "veryfast",
			"-b:v", "1500k",
			"-maxrate", "1800k",
			"-bufsize", "3000k",
			"-g", "50",
			"-profile:v", "baseline",
		}
	case "h264_amf":
		return []string{
			"-quality", "speed",
			"-b:v", "1500k",
			"-maxrate", "1800k",
			"-bufsize", "3000k",
			"-g", "50",
			"-profile:v", "baseline",
		}
	case "libx264":
		return []string{
			"-preset", "ultrafast",
			"-tune", "zerolatency",
			"-b:v", "1500k",
			"-maxrate", "1800k",
			"-bufsize", "3000k",
			"-g", "50",
			"-profile:v", "baseline",
			"-level", "3.1",
		}
	case "libopenh264":
		return []string{
			"-b:v", "1500k",
			"-g", "50",
			"-profile:v", "baseline",
		}
	case "h264_mf":
		return []string{
			"-b:v", "1500k",
			"-g", "50",
		}
	}
	return []string{"-b:v", "1500k", "-g", "50"}
}

// Handler — `/preview/{cameraID}/...` URL'larni HLS fayllarga yo'naltirilgan http.Handler.
// Wails' AssetServer.Handler sifatida ishlatiladi.
type Handler struct {
	PreviewDir string
}

// NewHandler — preview fayllarini beruvchi handler.
func NewHandler(previewDir string) *Handler {
	return &Handler{PreviewDir: previewDir}
}

// ServeHTTP — /preview/{id}/...  fayllarni qaytaradi.
func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	// URL shabloni: /preview/{cameraID}/index.m3u8 yoki /preview/{cameraID}/seg_N.ts
	path := strings.TrimPrefix(r.URL.Path, "/preview/")
	if path == r.URL.Path || path == "" {
		http.NotFound(w, r)
		return
	}

	// Yo'lda ".." bo'lmasligini ta'minlash (path traversal himoyasi)
	if strings.Contains(path, "..") {
		http.Error(w, "noto'g'ri yo'l", http.StatusBadRequest)
		return
	}

	fullPath := filepath.Join(h.PreviewDir, filepath.FromSlash(path))

	// MIME type
	if strings.HasSuffix(path, ".m3u8") {
		w.Header().Set("Content-Type", "application/vnd.apple.mpegurl")
		// HLS playlist tez-tez yangilanadi — cache'lamaslik
		w.Header().Set("Cache-Control", "no-cache, no-store, must-revalidate")
	} else if strings.HasSuffix(path, ".ts") {
		w.Header().Set("Content-Type", "video/mp2t")
		// .ts segmentlari o'zgarmaydi — kanal kesh qila oladi
		w.Header().Set("Cache-Control", "public, max-age=10")
	}

	w.Header().Set("Access-Control-Allow-Origin", "*")
	http.ServeFile(w, r, fullPath)
}
