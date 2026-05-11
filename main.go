// main.go — Wails entry point.
//
// React build'i embed.FS orqali binary ichiga qotirilgan.
// Tray icon ham embed orqali yuklanadi.
//
// "X" tugmasi bosilganda oyna yopilmaydi — tray'ga yashiriladi (OnBeforeClose).
package main

import (
	"context"
	"embed"
	_ "embed"
	"log"
	"net/http"
	"strings"

	"cam_to_you/internal/config"
	"cam_to_you/internal/preview"

	"github.com/wailsapp/wails/v2"
	"github.com/wailsapp/wails/v2/pkg/options"
	"github.com/wailsapp/wails/v2/pkg/options/assetserver"
	"github.com/wailsapp/wails/v2/pkg/options/windows"
	wailsruntime "github.com/wailsapp/wails/v2/pkg/runtime"
)

//go:embed all:frontend/dist
var assets embed.FS

//go:embed build/windows/icon.ico
var trayIcon []byte

func main() {
	app := NewApp(trayIcon)

	// AssetsHandler — preview HLS fayllarini /preview/{id}/... orqali beradi.
	// Boshqa yo'llar embed.FS (React build) tomonidan ko'rsatiladi.
	paths, _ := config.LoadPaths()
	previewHandler := preview.NewHandler(paths.PreviewsDir)

	err := wails.Run(&options.App{
		Title:     "Cam2You",
		Width:     1280,
		Height:    800,
		MinWidth:  900,
		MinHeight: 600,
		AssetServer: &assetserver.Options{
			Assets: assets,
			Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if strings.HasPrefix(r.URL.Path, "/preview/") {
					previewHandler.ServeHTTP(w, r)
					return
				}
				http.NotFound(w, r)
			}),
		},
		BackgroundColour: &options.RGBA{R: 18, G: 24, B: 36, A: 1},
		OnStartup:        app.startup,
		OnShutdown:       app.shutdown,

		// "X" bosilganda yopish O'RNIGA tray'ga yashirish.
		// Foydalanuvchi haqiqatan yopmoqchi bo'lsa, tray menudan "Chiqish"ni bossin.
		OnBeforeClose: func(ctx context.Context) (prevent bool) {
			wailsruntime.WindowHide(ctx)
			return true // close hodisasini bekor qilamiz
		},

		Windows: &windows.Options{
			WebviewIsTransparent:              false,
			WindowIsTranslucent:               false,
			DisableWindowIcon:                 false,
			DisableFramelessWindowDecorations: false,
		},

		Bind: []interface{}{
			app,
		},
	})

	if err != nil {
		log.Fatalf("Wails xatoligi: %v", err)
	}
}
