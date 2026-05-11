// app.go — Wails App struktura. React UI bilan barcha aloqalar shu yerdan o'tadi.
//
// Wails: shu strukturadagi public metodlar avtomatik React TypeScript funktsiyalariga
// aylanadi (wailsjs/go/main/App.ts).
package main

import (
	"context"
	"database/sql"
	"fmt"
	"log"
	"sync"
	"time"

	"cam_to_you/internal/camera"
	"cam_to_you/internal/config"
	"cam_to_you/internal/db"
	"cam_to_you/internal/ffmpeg"
	"cam_to_you/internal/models"
	"cam_to_you/internal/notify"
	"cam_to_you/internal/power"
	"cam_to_you/internal/preview"
	"cam_to_you/internal/stream"
	"cam_to_you/internal/tray"

	wailsruntime "github.com/wailsapp/wails/v2/pkg/runtime"
)

// App — Wails uchun asosiy struktura.
type App struct {
	ctx    context.Context
	paths  *config.Paths
	db     *sql.DB

	installer  *ffmpeg.Installer
	prober     *camera.Prober
	cameraSvc  *camera.Service
	streamSvc  *stream.Service
	streamMgr  *stream.Manager
	previewSvc *preview.Service
	notifier   *notify.Notifier
	power      *power.Keeper
	trayIcon   *tray.Tray

	// FFmpeg yo'llari (auto-installer topgan/o'rnatgan)
	ffmpegBin  string
	ffprobeBin string

	// trayIcon embed (icon.ico)
	trayIconBytes []byte

	// Hodisalar bo'sh subscriber
	once sync.Once
}

// NewApp — yangi App yaratadi (faqat boshlang'ich qiymatlar).
// Asosiy initsializatsiya startup()'da bo'ladi (Wails ctx kerak).
func NewApp(trayIconBytes []byte) *App {
	return &App{trayIconBytes: trayIconBytes}
}

// startup — Wails ilovasi ishga tushganda chaqiriladi.
// Bu yerda hamma kerakli servislarni initsializatsiya qilamiz.
func (a *App) startup(ctx context.Context) {
	a.ctx = ctx

	// 1) Yo'llarni sozlash
	paths, err := config.LoadPaths()
	if err != nil {
		a.fatal("Yo'llarni yuklab bo'lmadi: %v", err)
		return
	}
	a.paths = paths
	log.Printf("AppData: %s", paths.AppData)

	// 2) FFmpeg installer'ni tayyorlash (lekin hozir o'rnatmaymiz — bu UI orqali bo'ladi)
	a.installer = ffmpeg.New(paths.FFmpegBin, paths.FFprobeBin, paths.FFmpegDir)
	a.ffmpegBin, a.ffprobeBin = a.installer.Find()

	// 3) DB'ni ochish
	conn, err := db.Open(paths.DBFile)
	if err != nil {
		a.fatal("DB ochilmadi: %v", err)
		return
	}
	a.db = conn

	// 4) Servislar
	a.prober = camera.NewProber(a.ffprobeBin)
	a.cameraSvc = camera.New(a.db, a.prober)
	a.streamSvc = stream.NewService(a.db)

	// 5) Power keeper + notifier
	a.power = power.New()
	a.notifier = notify.New("Cam2You")

	// 6) Stream manager + Preview service
	a.streamMgr = stream.New(a.cameraSvc, a.streamSvc, a.ffmpegBin, a.power, a.notifier)
	a.previewSvc = preview.New(a.ffmpegBin, paths.PreviewsDir)

	// 7) Stream Manager hodisalarini React UI'ga uzatish
	events := make(chan stream.Event, 100)
	a.streamMgr.Subscribe(events)
	go a.forwardEvents(events)

	// 7b) Preview hodisalari — kamera live ko'rish uchun
	previewEvents := make(chan preview.Event, 100)
	a.previewSvc.Subscribe(previewEvents)
	go a.forwardPreviewEvents(previewEvents)

	// 8) System tray
	a.trayIcon = tray.New(tray.Callbacks{
		OnShow: func() {
			wailsruntime.WindowShow(a.ctx)
		},
		OnStartAll: func() {
			a.StartAllSavedStreams()
		},
		OnStopAll: func() {
			a.streamMgr.StopAll()
		},
		OnQuit: func() {
			a.streamMgr.StopAll()
			wailsruntime.Quit(a.ctx)
		},
	})
	a.trayIcon.Start(a.trayIconBytes)

	// 9) Toast: dastur ishga tushdi
	go func() {
		time.Sleep(500 * time.Millisecond)
		if !a.installer.IsInstalled() {
			_ = a.notifier.Notify("Cam2You", "FFmpeg topilmadi. Sozlamalardan o'rnatib oling.")
		}
	}()
}

// shutdown — Wails yopilayotganda chaqiriladi.
func (a *App) shutdown(ctx context.Context) {
	if a.streamMgr != nil {
		a.streamMgr.StopAll()
	}
	if a.previewSvc != nil {
		a.previewSvc.StopAll()
	}
	if a.power != nil {
		_ = a.power.Release()
	}
	if a.db != nil {
		a.db.Close()
	}
}

// forwardEvents — manager'dan kelgan hodisalarni Wails event bus'ga uzatadi.
// React'da `EventsOn("stream:event", ...)` bilan tinglanadi.
func (a *App) forwardEvents(events <-chan stream.Event) {
	for ev := range events {
		wailsruntime.EventsEmit(a.ctx, "stream:event", ev)
	}
}

// forwardPreviewEvents — preview servisidan kelgan hodisalarni UI'ga uzatadi.
// React'da `EventsOn("preview:event", ...)` bilan tinglanadi.
func (a *App) forwardPreviewEvents(events <-chan preview.Event) {
	for ev := range events {
		wailsruntime.EventsEmit(a.ctx, "preview:event", ev)
	}
}

func (a *App) fatal(format string, args ...interface{}) {
	msg := fmt.Sprintf(format, args...)
	log.Printf("FATAL: %s", msg)
	if a.ctx != nil {
		wailsruntime.MessageDialog(a.ctx, wailsruntime.MessageDialogOptions{
			Type:    wailsruntime.ErrorDialog,
			Title:   "Cam2You — xatolik",
			Message: msg,
		})
	}
}

// ============================================================================
//  React UI uchun bindings (Wails avtomatik TS funksiyalarga aylantiradi)
// ============================================================================

// --- FFmpeg / O'rnatish ---

// FFmpegStatus — FFmpeg holatini qaytaradi (UI'da Settings sahifasi uchun).
type FFmpegStatus struct {
	Installed bool   `json:"installed"`
	Path      string `json:"path"`
	Version   string `json:"version"`
}

// HardwareInfo — foydalanuvchi PC'sida qaysi encoderlar mavjudligi.
// UI'da Dashboard va Streams formada tavsiyalar berishda ishlatiladi.
type HardwareInfo struct {
	HasGPU         bool   `json:"hasGpu"`         // har qanday GPU encoder bormi?
	HasNVENC       bool   `json:"hasNvenc"`       // NVIDIA
	HasQuickSync   bool   `json:"hasQuickSync"`   // Intel
	HasAMF         bool   `json:"hasAmf"`         // AMD
	HasX264        bool   `json:"hasX264"`        // libx264 (GPL CPU)
	HasOpenH264    bool   `json:"hasOpenH264"`    // libopenh264 (LGPL CPU)
	HasMediaFound  bool   `json:"hasMediaFound"`  // Windows MF
	BestEncoder    string `json:"bestEncoder"`    // tavsiya etilgan encoder
	Recommendation string `json:"recommendation"` // foydalanuvchiga o'zbekcha tavsiya matn
}

// GetHardwareInfo — UI'ga tizim holati va tavsiyalar yuboradi.
func (a *App) GetHardwareInfo() HardwareInfo {
	encs := ffmpeg.DetectEncoders(a.ffmpegBin)
	info := HardwareInfo{
		HasNVENC:      encs.NVENC,
		HasQuickSync:  encs.QuickSync,
		HasAMF:        encs.AMF,
		HasX264:       encs.X264,
		HasOpenH264:   encs.OpenH264,
		HasMediaFound: encs.MediaFound,
		BestEncoder:   encs.BestH264(),
	}
	info.HasGPU = encs.NVENC || encs.QuickSync || encs.AMF

	switch {
	case encs.NVENC:
		info.Recommendation = "NVIDIA GPU mavjud — 1080p/1440p streamlar oson ko'tariladi"
	case encs.QuickSync:
		info.Recommendation = "Intel QuickSync GPU mavjud — 1080p stream qulay"
	case encs.AMF:
		info.Recommendation = "AMD GPU mavjud — 1080p stream qulay"
	case encs.X264 || encs.OpenH264:
		info.Recommendation = "GPU encoder yo'q — CPU bilan 720p stream tavsiya etiladi. " +
			"Hikvision sub-stream (/102) tanlasangiz, transcoding kerakmasdan ishlaydi"
	default:
		info.Recommendation = "Hech qanday H.264 encoder topilmadi — FFmpeg qayta o'rnating"
	}
	return info
}

func (a *App) GetFFmpegStatus() FFmpegStatus {
	bin, _ := a.installer.Find()
	status := FFmpegStatus{Installed: bin != "", Path: bin}
	if bin != "" {
		v, _ := a.installer.VerifyVersion(a.ctx)
		status.Version = v
	}
	return status
}

// InstallFFmpeg — FFmpeg'ni avtomatik yuklaydi va o'rnatadi.
// Davomida `ffmpeg:progress` hodisasi orqali UI'ga progres yuboriladi.
func (a *App) InstallFFmpeg() error {
	err := a.installer.EnsureInstalled(a.ctx, a.emitFFmpegProgress)
	if err != nil {
		return err
	}
	a.afterFFmpegInstalled()
	return nil
}

// BrowseFFmpegFile — fayl tanlash dialogini ochadi (foydalanuvchi o'zining ffmpeg.exe yoki ZIP'ini ko'rsatadi)
// va shu fayldan o'rnatadi.
func (a *App) BrowseFFmpegFile() error {
	path, err := wailsruntime.OpenFileDialog(a.ctx, wailsruntime.OpenDialogOptions{
		Title: "FFmpeg fayli (ffmpeg.exe yoki ZIP)",
		Filters: []wailsruntime.FileFilter{
			{DisplayName: "FFmpeg fayllari (*.exe, *.zip)", Pattern: "*.exe;*.zip"},
			{DisplayName: "Barchasi (*.*)", Pattern: "*.*"},
		},
	})
	if err != nil {
		return err
	}
	if path == "" {
		return nil // foydalanuvchi bekor qildi
	}

	if err := a.installer.InstallFromFile(path, a.emitFFmpegProgress); err != nil {
		return err
	}
	a.afterFFmpegInstalled()
	return nil
}

// emitFFmpegProgress — installer'dan kelgan progress hodisalarini UI'ga jo'natadi.
func (a *App) emitFFmpegProgress(stage string, percent float64, speedMBps float64, etaSec int, msg string) {
	wailsruntime.EventsEmit(a.ctx, "ffmpeg:progress", map[string]interface{}{
		"stage":     stage,
		"percent":   percent,
		"speedMBps": speedMBps,
		"etaSec":    etaSec,
		"message":   msg,
	})
}

// afterFFmpegInstalled — o'rnatish tugagandan keyin barcha xizmatlarda yo'llarni yangilash.
func (a *App) afterFFmpegInstalled() {
	a.ffmpegBin, a.ffprobeBin = a.installer.Find()
	a.prober.FFprobeBin = a.ffprobeBin
	if a.previewSvc != nil {
		a.previewSvc.SetFFmpegBin(a.ffmpegBin)
	}
	_ = a.notifier.Notify("Cam2You", "FFmpeg muvaffaqiyatli o'rnatildi")
}

// --- Preview (jonli ko'rish) ---

// StartPreview — kamera uchun HLS preview boshlaydi.
// Frontend keyinroq /preview/{cameraId}/index.m3u8 ni o'qiy oladi.
func (a *App) StartPreview(cameraID int64) error {
	if !a.installer.IsInstalled() {
		return fmt.Errorf("FFmpeg topilmadi — avval o'rnating")
	}
	cam, err := a.cameraSvc.Get(a.ctx, cameraID)
	if err != nil {
		return fmt.Errorf("kamera topilmadi: %w", err)
	}
	return a.previewSvc.Start(cameraID, cam.BuildRTSPURL())
}

// StopPreview — preview'ni to'xtatadi va fayllarni tozalaydi.
func (a *App) StopPreview(cameraID int64) error {
	return a.previewSvc.Stop(cameraID)
}

// IsPreviewActive — preview ishlayaptimi?
func (a *App) IsPreviewActive(cameraID int64) bool {
	return a.previewSvc.IsActive(cameraID)
}

// --- Cameralar CRUD ---

func (a *App) ListCameras() ([]*models.Camera, error) {
	return a.cameraSvc.List(a.ctx)
}

func (a *App) GetCamera(id int64) (*models.Camera, error) {
	return a.cameraSvc.Get(a.ctx, id)
}

func (a *App) CreateCamera(c models.Camera) (*models.Camera, error) {
	id, err := a.cameraSvc.Create(a.ctx, &c)
	if err != nil {
		return nil, err
	}
	c.ID = id
	return &c, nil
}

func (a *App) UpdateCamera(c models.Camera) error {
	return a.cameraSvc.Update(a.ctx, &c)
}

func (a *App) DeleteCamera(id int64) error {
	return a.cameraSvc.Delete(a.ctx, id)
}

// ProbeCameraSaved — saqlangan kamerani tekshiradi.
func (a *App) ProbeCameraSaved(id int64) (*camera.ProbeResult, error) {
	return a.cameraSvc.Probe(a.ctx, id)
}

// ProbeCamera — saqlanmagan kamerani (yangi yaratayotganda) tekshiradi.
func (a *App) ProbeCamera(c models.Camera) (*camera.ProbeResult, error) {
	return a.cameraSvc.ProbeAdhoc(a.ctx, &c)
}

// --- Streamlar CRUD + boshqaruv ---

func (a *App) ListStreams() ([]*models.Stream, error) {
	return a.streamSvc.List(a.ctx)
}

func (a *App) GetStream(id int64) (*models.Stream, error) {
	return a.streamSvc.Get(a.ctx, id)
}

func (a *App) CreateStream(s models.Stream) (*models.Stream, error) {
	id, err := a.streamSvc.Create(a.ctx, &s)
	if err != nil {
		return nil, err
	}
	s.ID = id
	return &s, nil
}

func (a *App) UpdateStream(s models.Stream) error {
	return a.streamSvc.Update(a.ctx, &s)
}

func (a *App) DeleteStream(id int64) error {
	// Avval to'xtatish, keyin o'chirish
	_ = a.streamMgr.Stop(id)
	return a.streamSvc.Delete(a.ctx, id)
}

// StartStream — stream'ni ishga tushiradi.
func (a *App) StartStream(id int64) error {
	if !a.installer.IsInstalled() {
		return fmt.Errorf("FFmpeg topilmadi — avval o'rnating")
	}
	return a.streamMgr.Start(a.ctx, id)
}

// StopStream — stream'ni to'xtatadi.
func (a *App) StopStream(id int64) error {
	return a.streamMgr.Stop(id)
}

// GetStreamStatus — bitta stream holati.
func (a *App) GetStreamStatus(id int64) models.StreamStatus {
	return a.streamMgr.Status(id)
}

// GetAllStreamStatus — barcha aktiv stream holatlari.
func (a *App) GetAllStreamStatus() map[int64]models.StreamStatus {
	return a.streamMgr.AllStatus()
}

// StartAllSavedStreams — barcha saqlangan stream'larni ishga tushiradi.
// (Tray menyusidan chaqiriladi)
func (a *App) StartAllSavedStreams() {
	streams, err := a.streamSvc.List(a.ctx)
	if err != nil {
		log.Printf("StartAllSavedStreams: %v", err)
		return
	}
	for _, s := range streams {
		_ = a.streamMgr.Start(a.ctx, s.ID)
	}
}

// --- Window ---

// HideWindow — oynani tray'ga yashiradi (React'dan "Yashirin rejim" tugmasi uchun).
func (a *App) HideWindow() {
	wailsruntime.WindowHide(a.ctx)
}

// Quit — dasturni butunlay yopadi.
func (a *App) Quit() {
	a.streamMgr.StopAll()
	wailsruntime.Quit(a.ctx)
}
