// Package stream — stream konfiguratsiyalarini boshqaradi (CRUD) va
// runtime manager bilan integratsiya qiladi.
package stream

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"cam_to_you/internal/ffmpeg"
	"cam_to_you/internal/models"
)

// Service — stream CRUD operatsiyalari.
type Service struct {
	db *sql.DB
}

func NewService(db *sql.DB) *Service {
	return &Service{db: db}
}

// List — barcha stream'larni qaytaradi.
func (s *Service) List(ctx context.Context) ([]*models.Stream, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, name, layout, camera_ids, quality, encoder,
		       audio_mode, audio_camera_index, platform, stream_key, custom_url,
		       auto_restart, max_restarts, restart_delay_ms,
		       created_at, updated_at
		FROM streams ORDER BY id
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []*models.Stream
	for rows.Next() {
		st, err := scanStream(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, st)
	}
	return out, rows.Err()
}

// Get — bitta stream'ni ID bo'yicha qaytaradi.
func (s *Service) Get(ctx context.Context, id int64) (*models.Stream, error) {
	row := s.db.QueryRowContext(ctx, `
		SELECT id, name, layout, camera_ids, quality, encoder,
		       audio_mode, audio_camera_index, platform, stream_key, custom_url,
		       auto_restart, max_restarts, restart_delay_ms,
		       created_at, updated_at
		FROM streams WHERE id = ?
	`, id)
	return scanStream(row)
}

// Create — yangi stream qo'shadi.
func (s *Service) Create(ctx context.Context, st *models.Stream) (int64, error) {
	if err := validate(st); err != nil {
		return 0, err
	}
	now := time.Now().UTC().Format(time.RFC3339)
	camIDsJSON, _ := json.Marshal(st.CameraIDs)
	res, err := s.db.ExecContext(ctx, `
		INSERT INTO streams (
			name, layout, camera_ids, quality, encoder,
			audio_mode, audio_camera_index, platform, stream_key, custom_url,
			auto_restart, max_restarts, restart_delay_ms,
			created_at, updated_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`, st.Name, string(st.Layout), string(camIDsJSON), string(st.Quality), string(st.Encoder),
		st.AudioMode, st.AudioCameraIndex, string(st.Platform), st.StreamKey, st.CustomURL,
		boolToInt(st.AutoRestart), st.MaxRestarts, st.RestartDelayMs,
		now, now)
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

// Update — mavjud stream'ni yangilaydi.
func (s *Service) Update(ctx context.Context, st *models.Stream) error {
	if st.ID == 0 {
		return errors.New("id majburiy")
	}
	if err := validate(st); err != nil {
		return err
	}
	now := time.Now().UTC().Format(time.RFC3339)
	camIDsJSON, _ := json.Marshal(st.CameraIDs)
	_, err := s.db.ExecContext(ctx, `
		UPDATE streams SET
			name=?, layout=?, camera_ids=?, quality=?, encoder=?,
			audio_mode=?, audio_camera_index=?, platform=?, stream_key=?, custom_url=?,
			auto_restart=?, max_restarts=?, restart_delay_ms=?,
			updated_at=?
		WHERE id=?
	`, st.Name, string(st.Layout), string(camIDsJSON), string(st.Quality), string(st.Encoder),
		st.AudioMode, st.AudioCameraIndex, string(st.Platform), st.StreamKey, st.CustomURL,
		boolToInt(st.AutoRestart), st.MaxRestarts, st.RestartDelayMs,
		now, st.ID)
	return err
}

// Delete — stream'ni o'chiradi.
func (s *Service) Delete(ctx context.Context, id int64) error {
	_, err := s.db.ExecContext(ctx, "DELETE FROM streams WHERE id = ?", id)
	return err
}

// validate — saqlashdan oldin asosiy tekshiruvlar.
// Stream key avtomatik tozalanadi: agar foydalanuvchi to'liq RTMP URL kiritsa,
// faqat key qismi (oxirgi `/` dan keyin) saqlanadi.
func validate(st *models.Stream) error {
	if st.Name == "" {
		return errors.New("nom bo'sh bo'lmasligi kerak")
	}
	if st.Layout == "" {
		st.Layout = ffmpeg.LayoutSingle
	}
	if len(st.CameraIDs) < st.Layout.CamerasNeeded() {
		return errors.New("kameralar yetarli emas: " + string(st.Layout) + " uchun " +
			itoa(st.Layout.CamerasNeeded()) + " kamera kerak")
	}
	if st.Quality == "" {
		st.Quality = models.Quality1080p30
	}
	if st.Encoder == "" {
		st.Encoder = ffmpeg.EncoderAuto
	}
	if st.Platform == models.PlatformCustom {
		if st.CustomURL == "" {
			return errors.New("custom platforma uchun RTMP URL kerak")
		}
	} else {
		// Avtomatik tozalash: trim + to'liq URL bo'lsa, key qismini ajratib olamiz
		st.StreamKey = sanitizeStreamKey(st.StreamKey)
		if st.StreamKey == "" {
			return errors.New("stream key bo'sh bo'lmasligi kerak")
		}
		// Shaklini tekshirish — masalan, "live2" qabul qilinmaydi
		if err := validateStreamKeyShape(st.StreamKey); err != nil {
			return err
		}
	}
	if st.RestartDelayMs == 0 {
		st.RestartDelayMs = 5000
	}
	return nil
}

// sanitizeStreamKey — foydalanuvchi to'liq RTMP URL kiritgan bo'lsa, key qismini ajratib oladi.
//
// Kiruvchi:                                        →  Chiquvchi:
//   "xxxx-xxxx-xxxx-xxxx"                          →  "xxxx-xxxx-xxxx-xxxx"
//   "rtmp://a.rtmp.youtube.com/live2/xxxx-xxxx-..."→  "xxxx-xxxx-..."
//   "rtmps://server/app/STREAM_KEY"                →  "STREAM_KEY"
//   "  xxxx-xxxx-... "                             →  "xxxx-xxxx-..."
func sanitizeStreamKey(key string) string {
	key = strings.TrimSpace(key)
	// Agar URL bo'lsa, oxirgi `/` dan keyin nima borligini olamiz
	if strings.HasPrefix(key, "rtmp://") || strings.HasPrefix(key, "rtmps://") {
		if idx := strings.LastIndex(key, "/"); idx != -1 && idx < len(key)-1 {
			key = key[idx+1:]
		}
	}
	return key
}

// Stream key sifatida noto'g'ri (path segment) qiymatlar. Bularni saqlashga ruxsat bermaymiz.
var suspiciousKeyValues = map[string]bool{
	"live": true, "live2": true, "live3": true,
	"app": true, "rtmp": true, "stream": true,
	"flv": true, "channel": true, "ingest": true,
}

// validateStreamKeyShape — key tashqi ko'rinishi bo'yicha tekshiruv.
// Maqsad: foydalanuvchini noaniq "I/O error" emas, aniq xato xabari bilan ogohlantirish.
func validateStreamKeyShape(key string) error {
	if len(key) < 10 {
		return fmt.Errorf("stream key juda qisqa (%d ta belgi) — YouTube Studio'dan to'liq key oling (~16-25 belgi, masalan: xxxx-xxxx-xxxx-xxxx)", len(key))
	}
	if suspiciousKeyValues[strings.ToLower(key)] {
		return fmt.Errorf("\"%s\" — bu serverning yo'l qismi, stream key emas. YouTube Studio → Go Live → Stream key qismidan to'liq qiymatni nusxalang", key)
	}
	// Faqat path segmenti bo'lib qolgan bo'lishi mumkin (no dashes, all letters)
	if !strings.ContainsAny(key, "-_") && len(key) < 15 {
		// Real YouTube key'lar deyarli har doim chiziqcha bilan
		// Cheklov bermayman — ehtimol custom platforma — lekin warning yetarli
	}
	return nil
}

type scanner interface {
	Scan(dest ...any) error
}

func scanStream(s scanner) (*models.Stream, error) {
	st := &models.Stream{}
	var camIDsJSON, layoutStr, qualityStr, encoderStr, platformStr string
	var autoRestart int
	var createdAt, updatedAt string

	err := s.Scan(&st.ID, &st.Name, &layoutStr, &camIDsJSON, &qualityStr, &encoderStr,
		&st.AudioMode, &st.AudioCameraIndex, &platformStr, &st.StreamKey, &st.CustomURL,
		&autoRestart, &st.MaxRestarts, &st.RestartDelayMs,
		&createdAt, &updatedAt)
	if err != nil {
		return nil, err
	}

	st.Layout = ffmpeg.Layout(layoutStr)
	st.Quality = models.Quality(qualityStr)
	st.Encoder = ffmpeg.Encoder(encoderStr)
	st.Platform = models.Platform(platformStr)
	st.AutoRestart = autoRestart == 1

	_ = json.Unmarshal([]byte(camIDsJSON), &st.CameraIDs)

	st.CreatedAt, _ = time.Parse(time.RFC3339, createdAt)
	st.UpdatedAt, _ = time.Parse(time.RFC3339, updatedAt)
	return st, nil
}

func boolToInt(b bool) int {
	if b {
		return 1
	}
	return 0
}

func itoa(i int) string {
	if i == 0 {
		return "0"
	}
	negative := false
	if i < 0 {
		negative = true
		i = -i
	}
	var buf [20]byte
	pos := len(buf)
	for i > 0 {
		pos--
		buf[pos] = byte('0' + i%10)
		i /= 10
	}
	if negative {
		pos--
		buf[pos] = '-'
	}
	return string(buf[pos:])
}
