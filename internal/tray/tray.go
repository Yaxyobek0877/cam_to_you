// Package tray — Windows system tray icon va menu.
//
// energye/systray kutubxonasi ishlatilgan — Wails v2 bilan yaxshi mos keladi.
// Tray goroutine alohida ishlaydi; lifecycle Wails app context'iga bog'lanmagan.
//
// Foydalanish:
//
//	t := tray.New(tray.Callbacks{
//	    OnShow:     func() { runtime.WindowShow(ctx) },
//	    OnQuit:     func() { runtime.Quit(ctx) },
//	    OnStartAll: func() { mgr.StartAll() },
//	    OnStopAll:  func() { mgr.StopAll() },
//	})
//	t.Start(iconBytes)
package tray

import (
	"github.com/energye/systray"
)

// Callbacks — tray menu hodisalari.
type Callbacks struct {
	OnShow     func() // "Dasturni ochish" bosilganda
	OnStartAll func() // "Hammasini boshlash"
	OnStopAll  func() // "Hammasini to'xtatish"
	OnQuit     func() // "Chiqish" — haqiqatda yopish
}

// Tray — system tray icon obyekt.
type Tray struct {
	cb Callbacks
}

// New — yangi Tray yaratadi.
func New(cb Callbacks) *Tray {
	return &Tray{cb: cb}
}

// Start — tray icon'ni ko'rsatadi va menu'ni yaratadi.
// Goroutine'da ishga tushadi, blok qilmaydi.
func (t *Tray) Start(iconBytes []byte) {
	go systray.Run(func() {
		systray.SetIcon(iconBytes)
		systray.SetTitle("Cam2You")
		systray.SetTooltip("Cam2You — Hikvision → YouTube streaming")

		mOpen := systray.AddMenuItem("Dasturni ochish", "Asosiy oynani ko'rsatish")
		mOpen.SetIcon(iconBytes)

		systray.AddSeparator()

		mStart := systray.AddMenuItem("Hammasini boshlash", "Saqlangan stream'larni ishga tushirish")
		mStop := systray.AddMenuItem("Hammasini to'xtatish", "Aktiv stream'larni to'xtatish")

		systray.AddSeparator()

		mQuit := systray.AddMenuItem("Chiqish", "Dasturni butunlay yopish")

		// Click handler'lar
		// energye/systray menu item'lar uchun Click() qabul qiladi
		mOpen.Click(func() {
			if t.cb.OnShow != nil {
				t.cb.OnShow()
			}
		})
		mStart.Click(func() {
			if t.cb.OnStartAll != nil {
				t.cb.OnStartAll()
			}
		})
		mStop.Click(func() {
			if t.cb.OnStopAll != nil {
				t.cb.OnStopAll()
			}
		})
		mQuit.Click(func() {
			if t.cb.OnQuit != nil {
				t.cb.OnQuit()
			}
			systray.Quit()
		})

		// Tray icon'ga chap tugma bosilganda — oynani ochish
		systray.SetOnClick(func(menu systray.IMenu) {
			if t.cb.OnShow != nil {
				t.cb.OnShow()
			}
		})
	}, func() {
		// Cleanup
	})
}

// Stop — tray icon'ni o'chiradi.
func (t *Tray) Stop() {
	systray.Quit()
}
