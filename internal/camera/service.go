// Package camera — kameralar CRUD va RTSP probe servisi.
//
// Service ham UI'dan, ham boshqa servislardan (stream manager) chaqiriladi.
// Barcha DB amaliyotlari shu yerda sodir bo'ladi — boshqa joydan SQL yozilmaydi.
package camera

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"cam_to_you/internal/models"
)

// Service — kamera CRUD operatsiyalari.
type Service struct {
	db     *sql.DB
	prober *Prober
}

// New — yangi service yaratadi.
func New(db *sql.DB, prober *Prober) *Service {
	return &Service{db: db, prober: prober}
}

// List — barcha kameralarni yaratilgan tartibida qaytaradi.
func (s *Service) List(ctx context.Context) ([]*models.Camera, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, name, vendor, host, port, username, password,
		       channel, use_sub_stream, raw_rtsp_url, created_at, updated_at
		FROM cameras
		ORDER BY id
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var cams []*models.Camera
	for rows.Next() {
		c, err := scanCamera(rows)
		if err != nil {
			return nil, err
		}
		cams = append(cams, c)
	}
	return cams, rows.Err()
}

// Get — bitta kamerani ID bo'yicha topadi. Topilmasa, sql.ErrNoRows qaytadi.
func (s *Service) Get(ctx context.Context, id int64) (*models.Camera, error) {
	row := s.db.QueryRowContext(ctx, `
		SELECT id, name, vendor, host, port, username, password,
		       channel, use_sub_stream, raw_rtsp_url, created_at, updated_at
		FROM cameras WHERE id = ?
	`, id)
	return scanCamera(row)
}

// Create — yangi kamera yozadi, ID'ni qaytaradi.
func (s *Service) Create(ctx context.Context, c *models.Camera) (int64, error) {
	if err := validate(c); err != nil {
		return 0, err
	}
	now := time.Now().UTC().Format(time.RFC3339)
	res, err := s.db.ExecContext(ctx, `
		INSERT INTO cameras (name, vendor, host, port, username, password,
		                    channel, use_sub_stream, raw_rtsp_url, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`, c.Name, c.Vendor, c.Host, c.Port, c.Username, c.Password,
		c.Channel, boolToInt(c.UseSubStream), c.RawRTSPURL, now, now)
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

// Update — kamerani yangilaydi.
func (s *Service) Update(ctx context.Context, c *models.Camera) error {
	if c.ID == 0 {
		return errors.New("id majburiy")
	}
	if err := validate(c); err != nil {
		return err
	}
	now := time.Now().UTC().Format(time.RFC3339)
	_, err := s.db.ExecContext(ctx, `
		UPDATE cameras SET
			name=?, vendor=?, host=?, port=?, username=?, password=?,
			channel=?, use_sub_stream=?, raw_rtsp_url=?, updated_at=?
		WHERE id=?
	`, c.Name, c.Vendor, c.Host, c.Port, c.Username, c.Password,
		c.Channel, boolToInt(c.UseSubStream), c.RawRTSPURL, now, c.ID)
	return err
}

// Delete — kamerani o'chiradi.
// Kelajakda: shu kamera ishlatilgan stream'lar bilan bog'liq tekshiruv qo'shilishi mumkin.
func (s *Service) Delete(ctx context.Context, id int64) error {
	_, err := s.db.ExecContext(ctx, "DELETE FROM cameras WHERE id = ?", id)
	return err
}

// Probe — kameraga ulanib, RTSP oqimini tasdiqlaydi.
func (s *Service) Probe(ctx context.Context, id int64) (*ProbeResult, error) {
	c, err := s.Get(ctx, id)
	if err != nil {
		return nil, err
	}
	if s.prober == nil {
		return nil, errors.New("prober mavjud emas (ffprobe topilmadi)")
	}
	return s.prober.Probe(ctx, c.BuildRTSPURL())
}

// ProbeAd-hoc — saqlanmagan kamera (foydalanuvchi UI'da yangi yaratayotganda) probe qiladi.
func (s *Service) ProbeAdhoc(ctx context.Context, c *models.Camera) (*ProbeResult, error) {
	if s.prober == nil {
		return nil, errors.New("prober mavjud emas (ffprobe topilmadi)")
	}
	return s.prober.Probe(ctx, c.BuildRTSPURL())
}

// GetMany — bir nechta kamerani ID bo'yicha topadi (stream uchun).
// Yo'qolgan ID'lar uchun xato qaytaradi.
func (s *Service) GetMany(ctx context.Context, ids []int64) ([]*models.Camera, error) {
	cams := make([]*models.Camera, 0, len(ids))
	for _, id := range ids {
		c, err := s.Get(ctx, id)
		if err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				return nil, fmt.Errorf("kamera %d topilmadi", id)
			}
			return nil, err
		}
		cams = append(cams, c)
	}
	return cams, nil
}

func validate(c *models.Camera) error {
	if c.Name == "" {
		return errors.New("nom bo'sh bo'lmasligi kerak")
	}
	if c.Vendor == models.VendorGeneric {
		if c.RawRTSPURL == "" {
			return errors.New("generic vendor uchun rawRtspUrl kerak")
		}
		return nil
	}
	if c.Host == "" {
		return errors.New("host bo'sh bo'lmasligi kerak")
	}
	if c.Port == 0 {
		c.Port = 554
	}
	if c.Channel == 0 {
		c.Channel = 1
	}
	return nil
}

// Scanner interfeysi — sql.Row va sql.Rows ikkalasiga ham mos
type scanner interface {
	Scan(dest ...any) error
}

func scanCamera(s scanner) (*models.Camera, error) {
	c := &models.Camera{}
	var useSub int
	var createdAt, updatedAt string
	err := s.Scan(&c.ID, &c.Name, &c.Vendor, &c.Host, &c.Port,
		&c.Username, &c.Password, &c.Channel, &useSub, &c.RawRTSPURL,
		&createdAt, &updatedAt)
	if err != nil {
		return nil, err
	}
	c.UseSubStream = useSub == 1
	c.CreatedAt, _ = time.Parse(time.RFC3339, createdAt)
	c.UpdatedAt, _ = time.Parse(time.RFC3339, updatedAt)
	return c, nil
}

func boolToInt(b bool) int {
	if b {
		return 1
	}
	return 0
}

// CameraInputJSON — frontend'dan keladigan kameraID'lar JSON formati uchun.
// Stream service'da ishlatiladi: camera_ids ustunini JSON deb saqlaydi.
//
// Helper'lar:
type CameraIDs []int64

func (c CameraIDs) MarshalJSON() ([]byte, error) {
	if c == nil {
		return []byte("[]"), nil
	}
	return json.Marshal([]int64(c))
}

func (c *CameraIDs) UnmarshalJSON(b []byte) error {
	var arr []int64
	if err := json.Unmarshal(b, &arr); err != nil {
		return err
	}
	*c = arr
	return nil
}
