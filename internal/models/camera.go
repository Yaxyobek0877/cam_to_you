// Package models — DB va Wails frontend o'rtasida uzatiladigan obyektlar.
//
// Frontend'da TypeScript tarjimasi avtomatik yaratiladi (`wails generate module`).
// Shu sababli barcha maydonlar **public** va **JSON tag**lari aniq bo'lishi shart.
package models

import "time"

// CameraVendor — kamera ishlab chiqaruvchisi. RTSP URL shabloni shunga bog'liq.
type CameraVendor string

const (
	VendorHikvision CameraVendor = "hikvision"
	VendorDahua     CameraVendor = "dahua"
	VendorGeneric   CameraVendor = "generic" // qo'lda URL kiritilgan
)

// Camera — bitta IP kamera haqida ma'lumot.
//
// RTSPURL avtomatik yasaladi (vendor + host/port/credentials/channel).
// Foydalanuvchi VendorGeneric'ni tanlasa, RawRTSPURL'dan to'g'ridan to'g'ri ishlatiladi.
type Camera struct {
	ID        int64        `json:"id"`
	Name      string       `json:"name"`           // foydalanuvchi ko'rsatadigan nomi
	Vendor    CameraVendor `json:"vendor"`
	Host      string       `json:"host"`           // IP yoki domain
	Port      int          `json:"port"`           // odatda 554
	Username  string       `json:"username"`
	Password  string       `json:"password"`       // DB'da shifrlangan saqlash kerak (V2)
	Channel   int          `json:"channel"`        // Hikvision'da 1, 2, 3...
	UseSubStream bool      `json:"useSubStream"`   // true bo'lsa /102, aks holda /101
	RawRTSPURL string      `json:"rawRtspUrl"`     // Generic vendor uchun

	CreatedAt time.Time `json:"createdAt"`
	UpdatedAt time.Time `json:"updatedAt"`

	// Hisoblangan maydonlar (DB'da saqlanmaydi)
	IsOnline   bool   `json:"isOnline,omitempty"`   // oxirgi probe natijasi
	LastProbed string `json:"lastProbed,omitempty"` // oxirgi probe vaqti (RFC3339)
}

// BuildRTSPURL — vendor shabloniga ko'ra to'liq RTSP URL yasaydi.
//
// Hikvision: rtsp://user:pass@host:port/Streaming/Channels/{ch}{streamType}
//
//	{streamType} = 01 (main, asosiy) yoki 02 (sub, ikkilamchi — past sifat)
//
// Dahua:     rtsp://user:pass@host:port/cam/realmonitor?channel={ch}&subtype={st}
//
//	{st} = 0 (main) yoki 1 (sub)
//
// Generic:   RawRTSPURL'ni o'zicha qaytaradi.
func (c *Camera) BuildRTSPURL() string {
	if c.Vendor == VendorGeneric {
		return c.RawRTSPURL
	}

	port := c.Port
	if port == 0 {
		port = 554
	}
	channel := c.Channel
	if channel == 0 {
		channel = 1
	}

	auth := ""
	if c.Username != "" {
		// TODO V2: URL-encode the password if it contains special chars
		auth = c.Username + ":" + c.Password + "@"
	}

	base := "rtsp://" + auth + c.Host + ":" + itoa(port)

	switch c.Vendor {
	case VendorHikvision:
		streamType := "01"
		if c.UseSubStream {
			streamType = "02"
		}
		return base + "/Streaming/Channels/" + itoa(channel) + streamType
	case VendorDahua:
		streamType := "0"
		if c.UseSubStream {
			streamType = "1"
		}
		return base + "/cam/realmonitor?channel=" + itoa(channel) + "&subtype=" + streamType
	}
	return base
}

func itoa(i int) string {
	// Kichik yordamchi — strconv'ni shu fayldan import qilmaslik uchun
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
