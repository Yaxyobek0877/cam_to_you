package models

import (
	"cam_to_you/internal/ffmpeg"
	"time"
)

// Quality — oldindan tayyorlangan video sifat profillari.
// Foydalanuvchi UI'da tanlaydi.
type Quality string

const (
	Quality720p30  Quality = "720p30"  // 2500 kbps
	Quality720p60  Quality = "720p60"  // 4500 kbps
	Quality1080p30 Quality = "1080p30" // 4500 kbps
	Quality1080p60 Quality = "1080p60" // 6000 kbps
	Quality1440p30 Quality = "1440p30" // 9000 kbps
)

// Resolve — Quality'dan width/height/fps/bitrate ni qaytaradi.
func (q Quality) Resolve() (width, height, fps, bitrateKbps int) {
	switch q {
	case Quality720p30:
		return 1280, 720, 30, 2500
	case Quality720p60:
		return 1280, 720, 60, 4500
	case Quality1080p60:
		return 1920, 1080, 60, 6000
	case Quality1440p30:
		return 2560, 1440, 30, 9000
	default: // 1080p30 default
		return 1920, 1080, 30, 4500
	}
}

// Platform — chiqish RTMP platformasi.
type Platform string

const (
	PlatformYouTube  Platform = "youtube"
	PlatformTwitch   Platform = "twitch"
	PlatformFacebook Platform = "facebook"
	PlatformCustom   Platform = "custom" // RTMP URL to'liq qo'lda
)

// RTMPBaseURL — platformaning RTMP ingest URL'sini qaytaradi (StreamKey'sis).
func (p Platform) RTMPBaseURL() string {
	switch p {
	case PlatformYouTube:
		return "rtmp://a.rtmp.youtube.com/live2/"
	case PlatformTwitch:
		// Twitch uchun foydalanuvchi yaqin serverni tanlashi mumkin, hozircha default
		return "rtmp://live.twitch.tv/app/"
	case PlatformFacebook:
		return "rtmps://live-api-s.facebook.com:443/rtmp/"
	}
	return ""
}

// Stream — foydalanuvchi tomonidan yaratilgan stream konfiguratsiyasi.
// "Stream" — bu konfiguratsiya, "ishlash" emas. Ishlash holati — StreamStatus'da.
type Stream struct {
	ID     int64  `json:"id"`
	Name   string `json:"name"` // foydalanuvchi ko'rsatadigan nomi

	// Layout va kameralar
	Layout    ffmpeg.Layout `json:"layout"`
	CameraIDs []int64       `json:"cameraIds"` // tartib muhim — birinchi = asosiy

	// Video sifati
	Quality Quality `json:"quality"`

	// Kodlash
	Encoder ffmpeg.Encoder `json:"encoder"` // auto, NVENC, QSV, x264, copy

	// Audio
	AudioMode         string `json:"audioMode"`         // "first", "muted", "index"
	AudioCameraIndex  int    `json:"audioCameraIndex"`  // "index" rejimida qaysi kamera

	// Chiqish
	Platform   Platform `json:"platform"`
	StreamKey  string   `json:"streamKey"`   // YouTube stream key (DB'da shifrlangan saqlash kerak)
	CustomURL  string   `json:"customUrl"`   // PlatformCustom uchun to'liq RTMP URL

	// Auto-restart sozlamalari
	AutoRestart    bool `json:"autoRestart"`    // uzilsa qayta ulanish
	MaxRestarts    int  `json:"maxRestarts"`    // ketma-ket muvaffaqiyatsiz qayta urinishlar chegarasi (0 = cheksiz)
	RestartDelayMs int  `json:"restartDelayMs"` // qayta urinishlar oralig'i (ms)

	CreatedAt time.Time `json:"createdAt"`
	UpdatedAt time.Time `json:"updatedAt"`
}

// FullRTMPURL — to'liq RTMP destination URL'ni qaytaradi.
func (s *Stream) FullRTMPURL() string {
	if s.Platform == PlatformCustom {
		return s.CustomURL
	}
	return s.Platform.RTMPBaseURL() + s.StreamKey
}

// StreamStatus — ishlash holati (DB'da saqlanmaydi, runtime'da).
type StreamStatus struct {
	StreamID    int64     `json:"streamId"`
	State       string    `json:"state"`       // idle, starting, running, stopping, stopped, error
	Uptime      int64     `json:"uptime"`      // sekundlarda
	LastError   string    `json:"lastError"`
	StartedAt   time.Time `json:"startedAt,omitempty"`
	RestartCount int      `json:"restartCount"`
}
