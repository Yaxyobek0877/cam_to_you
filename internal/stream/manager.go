// Package stream — manager: bir nechta FFmpeg process'ini boshqaradi.
//
// Manager javobgar:
//   - Stream'ni ishga tushirish (Start)
//   - Stream'ni to'xtatish (Stop)
//   - FFmpeg uzilsa avtomatik qayta urinish (AutoRestart bayrog'i)
//   - Holatni har bir UI obunachisiga uzatish (Events)
//   - Aktiv streamlar bo'lsa, tizim uxlamasligi uchun PowerKeeper'ni qo'shib qo'yish
package stream

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"sync"
	"time"

	"cam_to_you/internal/camera"
	"cam_to_you/internal/ffmpeg"
	"cam_to_you/internal/models"
)

// PowerKeeper — tizim uxlamasligini ta'minlovchi interfeys.
// Aktiv stream bo'lganda Hold(), hammasi to'xtaganda Release() chaqiriladi.
// power paketi shu interfeysni amalga oshiradi.
type PowerKeeper interface {
	Hold() error
	Release() error
}

// Notifier — UI'ga toast bildirishnomalari yuborish uchun.
// notify paketi shu interfeysni amalga oshiradi.
type Notifier interface {
	Notify(title, message string) error
}

// EventType — manager hodisalari turlari.
type EventType string

const (
	EventStateChange EventType = "state_change"
	EventLog         EventType = "log"
	EventError       EventType = "error"
	EventRestart     EventType = "restart"
	// EventExitReason — FFmpeg chiqqanidan keyin oxirgi log qatorlarini ko'rsatadi.
	// Hatto toza chiqishda (exit 0) ham foydalanuvchi sababini tushunsin uchun.
	EventExitReason EventType = "exit_reason"
	// EventProgress — FFmpeg'ning periodik statistikasi (frame=, fps=, bitrate=).
	// "Running" holatda foydalanuvchi stream haqiqatdan ishlayotganini bilishi uchun MUHIM.
	// Aks holda u 2 ta warning ko'rib "ulanish yo'q" deb o'ylaydi.
	EventProgress EventType = "progress"
	// EventLive — birinchi frame YouTube'ga uchganida bir martalik tasdiq.
	// "✅ Stream YouTube'ga ulanmoqda" tipidagi xabarni keltirib chiqaradi.
	EventLive EventType = "live"
)

// Event — manager'dan UI'ga uzatiladigan hodisalar.
type Event struct {
	Type     EventType   `json:"type"`
	StreamID int64       `json:"streamId"`
	Payload  interface{} `json:"payload,omitempty"`
}

// Manager — barcha aktiv stream'larni boshqaradi.
type Manager struct {
	cameraSvc   *camera.Service
	streamSvc   *Service
	ffmpegBin   string
	power       PowerKeeper
	notifier    Notifier

	mu          sync.RWMutex
	runners     map[int64]*streamRunner // streamID -> runner
	subscribers []chan Event
}

// streamRunner — bitta stream uchun runtime ma'lumotlari.
type streamRunner struct {
	stream       *models.Stream
	runner       *ffmpeg.Runner
	cancel       context.CancelFunc // supervisor goroutine'ni to'xtatadi
	restartCount int
	startedAt    time.Time
}

// New — yangi Manager yaratadi.
func New(camSvc *camera.Service, streamSvc *Service, ffmpegBin string, power PowerKeeper, notifier Notifier) *Manager {
	return &Manager{
		cameraSvc: camSvc,
		streamSvc: streamSvc,
		ffmpegBin: ffmpegBin,
		power:     power,
		notifier:  notifier,
		runners:   make(map[int64]*streamRunner),
	}
}

// Subscribe — Manager hodisalarini ko'rsatilgan kanalga yuboradi.
// Qaytarilgan unsubscribe funksiyasini chaqirish bilan obunani bekor qilish mumkin.
func (m *Manager) Subscribe(ch chan Event) (unsubscribe func()) {
	m.mu.Lock()
	m.subscribers = append(m.subscribers, ch)
	m.mu.Unlock()
	return func() {
		m.mu.Lock()
		defer m.mu.Unlock()
		for i, c := range m.subscribers {
			if c == ch {
				m.subscribers = append(m.subscribers[:i], m.subscribers[i+1:]...)
				return
			}
		}
	}
}

// emit — hodisani barcha subscriber'larga yuboradi (non-blocking).
func (m *Manager) emit(e Event) {
	m.mu.RLock()
	subs := make([]chan Event, len(m.subscribers))
	copy(subs, m.subscribers)
	m.mu.RUnlock()
	for _, ch := range subs {
		select {
		case ch <- e:
		default:
		}
	}
}

// Start — stream'ni ishga tushiradi va supervisor'ni boshlaydi.
// Allaqachon ishlayotgan bo'lsa, xato qaytaradi.
func (m *Manager) Start(ctx context.Context, streamID int64) error {
	m.mu.Lock()
	if _, exists := m.runners[streamID]; exists {
		m.mu.Unlock()
		return errors.New("stream allaqachon ishlamoqda")
	}
	m.mu.Unlock()

	// Stream konfiguratsiyasini olamiz
	st, err := m.streamSvc.Get(ctx, streamID)
	if err != nil {
		return fmt.Errorf("stream topilmadi: %w", err)
	}

	// Stream key shaklini tekshiramiz — eski bazada noto'g'ri saqlanganlarni
	// FFmpeg'ga bermaslik uchun (aniq xato xabari beriladi).
	if st.Platform != models.PlatformCustom {
		if err := ValidateStreamKeyShape(st.StreamKey); err != nil {
			return err
		}
	}

	// Kameralarni olamiz
	cams, err := m.cameraSvc.GetMany(ctx, st.CameraIDs)
	if err != nil {
		return fmt.Errorf("kameralar topilmadi: %w", err)
	}

	// FFmpeg konfiguratsiyasini quramiz
	cfg := buildFFmpegConfig(st, cams)

	// Encoder'ni resolve qilamiz (auto → mavjud GPU, yoki fallback)
	encs := ffmpeg.DetectEncoders(m.ffmpegBin)
	resolvedEnc, err := ffmpeg.ResolveEncoder(cfg.Encoder, encs)
	if err != nil {
		return fmt.Errorf("encoder topib bo'lmadi: %w", err)
	}
	cfg.Encoder = resolvedEnc

	args, err := ffmpeg.Build(cfg)
	if err != nil {
		return fmt.Errorf("ffmpeg buyruq qurib bo'lmadi: %w", err)
	}

	// Runner yaratamiz
	supCtx, supCancel := context.WithCancel(context.Background())
	sr := &streamRunner{
		stream:    st,
		cancel:    supCancel,
		startedAt: time.Now(),
	}

	m.mu.Lock()
	m.runners[streamID] = sr
	count := len(m.runners)
	m.mu.Unlock()

	// Birinchi aktiv stream bo'lsa, tizim uxlashini bloklaymiz
	if count == 1 && m.power != nil {
		_ = m.power.Hold()
	}

	// Supervisor'ni alohida goroutine'da ishga tushiramiz
	go m.supervise(supCtx, sr, args)

	if m.notifier != nil {
		_ = m.notifier.Notify("Stream boshlandi", st.Name)
	}

	return nil
}

// supervise — FFmpeg'ni ishga tushiradi va kerak bo'lsa qayta urinadi.
func (m *Manager) supervise(ctx context.Context, sr *streamRunner, args []string) {
	defer m.cleanup(sr.stream.ID)

	for {
		runner := ffmpeg.NewRunner(
			fmt.Sprintf("stream-%d", sr.stream.ID),
			m.ffmpegBin,
			args,
		)

		m.mu.Lock()
		sr.runner = runner
		m.mu.Unlock()

		// Log subscription: FFmpeg log'larini hodisa sifatida UI'ga uzatamiz
		logCh := make(chan ffmpeg.LogLine, 100)
		unsub := runner.Subscribe(logCh)
		go m.forwardLogs(sr.stream.ID, logCh)

		// Boshlash
		m.emit(Event{Type: EventStateChange, StreamID: sr.stream.ID, Payload: ffmpeg.StateStarting})
		if err := runner.Start(ctx); err != nil {
			m.emit(Event{Type: EventError, StreamID: sr.stream.ID, Payload: err.Error()})
			unsub()
			close(logCh)
			if !sr.stream.AutoRestart {
				return
			}
		} else {
			m.emit(Event{Type: EventStateChange, StreamID: sr.stream.ID, Payload: ffmpeg.StateRunning})

			// Process chiqishini yoki tashqi ctx bekor qilinishini kutish.
			// MUHIM: ctx.Done — bu foydalanuvchi/manager Stop() chaqirgan.
			// runner.Done — FFmpeg o'zi chiqdi (xato yoki toza, qaysiy bo'lmasin —
			// foydalanuvchi xohlamagan, restart qilamiz).
			select {
			case <-runner.Done():
				// FFmpeg o'zi chiqdi — restartable
			case <-ctx.Done():
				// Foydalanuvchi/Manager Stop() chaqirdi
				_ = runner.Stop(5 * time.Second)
				unsub()
				close(logCh)
				m.emit(Event{Type: EventStateChange, StreamID: sr.stream.ID, Payload: ffmpeg.StateStopped})
				return
			}

			unsub()
			close(logCh)

			finalState := runner.State()
			m.emit(Event{Type: EventStateChange, StreamID: sr.stream.ID, Payload: finalState})

			// DIAGNOSTIKA: chiqish sababini ko'rsatamiz — toza chiqishda ham
			// FFmpeg'ning oxirgi qatorlarini foydalanuvchiga ko'rsatish kerak.
			// Aks holda "stopped → restart" loop'ida nima xato bo'layotgani bilinmaydi.
			m.emitExitReason(sr.stream.ID, runner)

			// Bu joyga yetishimiz uchun: ctx.Done bo'lmadi, demak foydalanuvchi
			// to'xtatmagan. FFmpeg o'zi chiqdi (xato yoki toza). AutoRestart bo'lsa
			// qayta urinamiz — toza chiqish ham kutilmagan hisoblanadi.
			if !sr.stream.AutoRestart {
				if m.notifier != nil {
					reason := runner.LastError()
					if reason == "" {
						reason = "FFmpeg kutilmagan ravishda chiqdi"
					}
					_ = m.notifier.Notify("Stream to'xtadi", sr.stream.Name+": "+reason)
				}
				return
			}
		}

		// Restart logikasi
		sr.restartCount++
		if sr.stream.MaxRestarts > 0 && sr.restartCount > sr.stream.MaxRestarts {
			if m.notifier != nil {
				_ = m.notifier.Notify("Stream to'xtatildi",
					fmt.Sprintf("%s: maksimal qayta urinishlar (%d) tugadi",
						sr.stream.Name, sr.stream.MaxRestarts))
			}
			return
		}

		m.emit(Event{Type: EventRestart, StreamID: sr.stream.ID, Payload: sr.restartCount})

		// Restart delay strategiyasi:
		//   - 1-urinish: 3 sek (kamera RTSP sessiyani bo'shatishi uchun MAJBURIY)
		//   - 2-10: 3 sek qatorida
		//   - 11+: exponential backoff (3 → 6 → 12 → ... → 5 min)
		//
		// MUHIM: Hikvision kameralar oldingi sessiyani 2-3 sek davomida tutib turadi.
		// Darrov qayta urinishi yangi sessiya emas, eski qoldiqni urgan kabi —
		// kamera ham yangi ulanishni yopib qo'yadi. 3 sek minimum.
		baseDelay := 3 * time.Second
		delay := baseDelay
		if sr.restartCount > 10 {
			// 11-urinishdan boshlab exponential — muammo doimiy bo'lsa
			extra := sr.restartCount - 10
			for i := 0; i < extra && delay < 5*time.Minute; i++ {
				delay *= 2
			}
		}
		if delay > 5*time.Minute {
			delay = 5 * time.Minute
		}

		select {
		case <-time.After(delay):
			// urinishni davom ettiramiz
		case <-ctx.Done():
			return
		}
	}
}

// emitExitReason — FFmpeg chiqqanidan keyin oxirgi 8 ta log qatorini UI'ga yuboradi.
// Foydalanuvchi nima sababdan stream uzilganini ko'rishi uchun MUHIM —
// runner.lastErr toza chiqishda bo'sh qoladi va sabab yashirinib qoladi.
//
// Bonus: log'larda mashhur xato patternlarini aniqlab, foydalanuvchiga
// aniq tavsiya beradi (masalan, RTSP EOF → kamera sessiya cheklovi tavsiyasi).
func (m *Manager) emitExitReason(streamID int64, runner *ffmpeg.Runner) {
	logs := runner.RecentLogs()
	if len(logs) == 0 {
		return
	}
	// Oxirgi 8 qator yetarli — odatda chiqish sababi shu yerda
	const maxLines = 8
	start := len(logs) - maxLines
	if start < 0 {
		start = 0
	}
	lines := make([]map[string]interface{}, 0, len(logs)-start)
	allText := ""
	for i := start; i < len(logs); i++ {
		lines = append(lines, map[string]interface{}{
			"time":    logs[i].Time.Format(time.RFC3339),
			"level":   string(logs[i].Level),
			"message": logs[i].Message,
		})
		allText += logs[i].Message + "\n"
	}
	exitCode := runner.ExitCode()

	// Mashhur xato patternlarini aniqlash — foydalanuvchiga aniq tavsiya beradi
	hint := detectExitHint(allText, exitCode)

	m.emit(Event{
		Type:     EventExitReason,
		StreamID: streamID,
		Payload: map[string]interface{}{
			"exitCode": exitCode,
			"state":    string(runner.State()),
			"lines":    lines,
			"hint":     hint, // Foydalanuvchi uchun tushunarli sabab+yechim
		},
	})
}

// detectExitHint — log matnida mashhur xato patternlarini topib,
// foydalanuvchiga ona tilida aniq tavsiya beradi.
func detectExitHint(logText string, exitCode int) string {
	lower := strings.ToLower(logText)
	switch {
	case strings.Contains(lower, "failed reading rtsp data: end of file"),
		strings.Contains(lower, "rtsp: end of file"):
		return "🎥 Kamera RTSP sessiyani yopdi (EOF). Bu odatda Hikvision sessiya " +
			"cheklovi. YECHIM: kamera web UI → Configuration → Video/Audio → " +
			"Sub Stream → Video Encoding'ni H.264 ga o'zgartiring (H.265 dan). " +
			"Keyin Cam2You'da Encoder = Copy tanlang."
	case strings.Contains(lower, "401 unauthorized"),
		strings.Contains(lower, "authentication failed"):
		return "🔐 RTSP autentifikatsiya muvaffaqiyatsiz. Kamera login/parolini tekshiring."
	case strings.Contains(lower, "connection refused"):
		return "🌐 Kameraga ulanib bo'lmadi. IP-manzil va port to'g'rimi tekshiring."
	case strings.Contains(lower, "connection reset by peer"):
		return "🔌 Tarmoq ulanishi uzildi. Wi-Fi/LAN holatini tekshiring."
	case strings.Contains(lower, "i/o error"),
		strings.Contains(lower, "error opening output"):
		return "📡 RTMP push muvaffaqiyatsiz. Stream key noto'g'ri yoki YouTube'da broadcast yoqilmagan."
	case strings.Contains(lower, "no such file or directory"):
		return "📁 FFmpeg yo'li yoki kamera URL'i noto'g'ri."
	case strings.Contains(lower, "broken pipe"):
		return "💔 RTMP serveri ulanishni yopdi. YouTube boshqa joydan stream qabul qilayotgan bo'lishi mumkin."
	case strings.Contains(lower, "nvenc"):
		if strings.Contains(lower, "session limit") || strings.Contains(lower, "max sessions") {
			return "🎮 NVIDIA NVENC sessiya cheklovi. Boshqa stream/yozuvni yoping yoki Encoder = Copy ishlating."
		}
	}
	if exitCode == 0 {
		return "ℹ️ FFmpeg toza chiqdi (exit=0). Odatda kamera RTSP sessiyani yopgan. " +
			"Agar 10+ urinishdan keyin ham davom etsa, kamera sozlamalarini tekshiring."
	}
	return ""
}

// statsRegex — FFmpeg'ning periodik statistika qatorini tahlil qiladi.
// Misol: "frame=  120 fps= 30 q=-0.0 size=    500KiB time=00:00:04.00 bitrate=1024.5kbits/s speed=1.0x"
// Stream haqiqatdan ishlayotganini foydalanuvchiga ko'rsatish uchun ishlatamiz.
var statsRegex = regexp.MustCompile(`frame=\s*(\d+)\s+fps=\s*([\d.]+).*?size=\s*(\S+).*?time=(\S+)\s+bitrate=\s*([\d.]+)kbits/s`)

// forwardLogs — FFmpeg log'larini Manager hodisalariga aylantiradi.
//
// Logikasi:
//   - Warning/Error — har biri darrov yuboriladi (foydalanuvchi tafsilotini ko'radi)
//   - Info darajadagi statistika qatorlari ("frame=") — parse qilinib, EventProgress
//     orqali yuboriladi. Birinchi shunday qator EventLive'ni keltirib chiqaradi —
//     foydalanuvchiga "✅ Stream ishlamoqda" ni darrov ko'rsatadi.
//   - Boshqa info qatorlari — yashirin (ko'p va asosan e'tiborga olinmaydi)
func (m *Manager) forwardLogs(streamID int64, logCh <-chan ffmpeg.LogLine) {
	var (
		firstFrameSeen bool
		lastProgressAt time.Time
	)
	const progressInterval = 5 * time.Second // har 5 sek'da bittadan ko'p yubormaslik

	for ll := range logCh {
		// Info darajada — statistika qatorimi tekshiramiz
		if ll.Level == ffmpeg.LogInfo {
			match := statsRegex.FindStringSubmatch(ll.Message)
			if match == nil {
				continue // oddiy info qatori — yashiramiz
			}
			// Statistika topildi: frames, fps, size, time, bitrate
			frames, fps, size, timeCode, bitrate := match[1], match[2], match[3], match[4], match[5]

			// Birinchi frame: foydalanuvchini xabardor qilamiz
			if !firstFrameSeen {
				firstFrameSeen = true
				m.emit(Event{
					Type:     EventLive,
					StreamID: streamID,
					Payload: map[string]interface{}{
						"message": "✅ Stream ishlamoqda — YouTube'ga ma'lumot yuborilmoqda",
						"frames":  frames,
						"fps":     fps,
					},
				})
			}

			// Cooldown: 5 sek'da bir progress, log'ni spam qilmaslik
			if !lastProgressAt.IsZero() && ll.Time.Sub(lastProgressAt) < progressInterval {
				continue
			}
			lastProgressAt = ll.Time

			m.emit(Event{
				Type:     EventProgress,
				StreamID: streamID,
				Payload: map[string]interface{}{
					"frames":  frames,
					"fps":     fps,
					"size":    size,
					"time":    timeCode,
					"bitrate": bitrate + "kbps",
				},
			})
			continue
		}
		// Warning yoki Error — to'g'ridan to'g'ri yuboriladi
		m.emit(Event{
			Type:     EventLog,
			StreamID: streamID,
			Payload: map[string]interface{}{
				"time":    ll.Time.Format(time.RFC3339),
				"level":   string(ll.Level),
				"message": ll.Message,
			},
		})
	}
}

// cleanup — supervisor tugaganida runner'ni xaritadan o'chiradi va power'ni bo'shatadi.
func (m *Manager) cleanup(streamID int64) {
	m.mu.Lock()
	delete(m.runners, streamID)
	remaining := len(m.runners)
	m.mu.Unlock()
	if remaining == 0 && m.power != nil {
		_ = m.power.Release()
	}
}

// Stop — stream'ni to'xtatadi.
func (m *Manager) Stop(streamID int64) error {
	m.mu.Lock()
	sr, ok := m.runners[streamID]
	m.mu.Unlock()
	if !ok {
		return errors.New("stream ishlamayapti")
	}
	sr.cancel() // supervisor'ni signallaymiz, u runner'ni Stop qiladi
	return nil
}

// StopAll — barcha aktiv stream'larni to'xtatadi.
func (m *Manager) StopAll() {
	m.mu.RLock()
	ids := make([]int64, 0, len(m.runners))
	for id := range m.runners {
		ids = append(ids, id)
	}
	m.mu.RUnlock()
	for _, id := range ids {
		_ = m.Stop(id)
	}
}

// Status — bitta stream'ning joriy holatini qaytaradi.
func (m *Manager) Status(streamID int64) models.StreamStatus {
	m.mu.RLock()
	defer m.mu.RUnlock()
	sr, ok := m.runners[streamID]
	if !ok {
		return models.StreamStatus{
			StreamID: streamID,
			State:    string(ffmpeg.StateIdle),
		}
	}
	status := models.StreamStatus{
		StreamID:     streamID,
		State:        string(ffmpeg.StateIdle),
		RestartCount: sr.restartCount,
		StartedAt:    sr.startedAt,
	}
	if sr.runner != nil {
		status.State = string(sr.runner.State())
		status.Uptime = int64(sr.runner.Uptime().Seconds())
		status.LastError = sr.runner.LastError()
	}
	return status
}

// AllStatus — barcha aktiv stream'lar holati.
func (m *Manager) AllStatus() map[int64]models.StreamStatus {
	m.mu.RLock()
	defer m.mu.RUnlock()
	out := make(map[int64]models.StreamStatus, len(m.runners))
	for id, sr := range m.runners {
		st := models.StreamStatus{
			StreamID:     id,
			State:        string(ffmpeg.StateIdle),
			RestartCount: sr.restartCount,
			StartedAt:    sr.startedAt,
		}
		if sr.runner != nil {
			st.State = string(sr.runner.State())
			st.Uptime = int64(sr.runner.Uptime().Seconds())
			st.LastError = sr.runner.LastError()
		}
		out[id] = st
	}
	return out
}

// buildFFmpegConfig — Stream va Camera'lardan FFmpeg StreamConfig quradi.
func buildFFmpegConfig(st *models.Stream, cams []*models.Camera) ffmpeg.StreamConfig {
	cfg := ffmpeg.StreamConfig{
		Layout:  st.Layout,
		Encoder: st.Encoder,
		Audio: ffmpeg.AudioMode{
			Mode:         st.AudioMode,
			CameraSource: st.AudioCameraIndex,
		},
		RTMPURL: st.FullRTMPURL(),
	}

	cfg.Width, cfg.Height, cfg.FPS, cfg.BitrateKbps = st.Quality.Resolve()

	for _, c := range cams {
		cfg.Cameras = append(cfg.Cameras, ffmpeg.CameraInput{
			RTSPURL: c.BuildRTSPURL(),
			UseTCP:  true, // Hikvision uchun har doim TCP
		})
	}
	return cfg
}
