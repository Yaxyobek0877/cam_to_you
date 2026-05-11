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

		// Restart delay: birinchi 10 marta — juda qisqa (1s), keyin exponential backoff
		// Kamera sessiya cheklovlari uchun tez restart — YouTube broadcast'ni saqlab qoladi.
		baseDelay := 1 * time.Second
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
	for i := start; i < len(logs); i++ {
		lines = append(lines, map[string]interface{}{
			"time":    logs[i].Time.Format(time.RFC3339),
			"level":   string(logs[i].Level),
			"message": logs[i].Message,
		})
	}
	exitCode := runner.ExitCode()
	m.emit(Event{
		Type:     EventExitReason,
		StreamID: streamID,
		Payload: map[string]interface{}{
			"exitCode": exitCode,
			"state":    string(runner.State()),
			"lines":    lines,
		},
	})
}

// forwardLogs — FFmpeg log'larini Manager hodisalariga aylantiradi.
func (m *Manager) forwardLogs(streamID int64, logCh <-chan ffmpeg.LogLine) {
	for ll := range logCh {
		// Faqat warning va undan yuqori loglarni UI'ga jo'natamiz (info ko'p)
		if ll.Level == ffmpeg.LogInfo {
			continue
		}
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
