//go:build windows

// Package power — tizim uxlamasligini boshqaradi.
//
// Windows'da SetThreadExecutionState API chaqiriladi:
//   - ES_CONTINUOUS         — flag o'rnatilgan holda qoladi
//   - ES_SYSTEM_REQUIRED    — tizim uxlamaydi (lekin ekran o'chishi mumkin)
//   - ES_AWAYMODE_REQUIRED  — "away mode" — ekran o'chsa ham fon'da ishlaydi
//
// "Ekran o'chsa ham stream to'xtamasin" talabini bajarish uchun ES_SYSTEM_REQUIRED kifoya.
// Ekran o'chishi (display sleep) FFmpeg'ga ta'sir qilmaydi — faqat tizim uxlashi (sleep) muammo.
package power

import (
	"sync"

	"golang.org/x/sys/windows"
)

// Windows API konstantalari
const (
	esSystemRequired = 0x00000001
	esContinuous     = 0x80000000
	esAwayMode       = 0x00000040
)

// Keeper — tizim uxlamasligini ta'minlovchi obyekt.
//
// Hold() — uxlashni bloklaydi. Bir necha marta chaqirilsa, reference count oshadi.
// Release() — bitta hold'ni qaytaradi. Counter 0 bo'lganda flag o'chiriladi.
//
// Bu shu uchun kerak: bir necha stream aktiv bo'lishi mumkin. Har biri o'z Hold()'ni qiladi.
type Keeper struct {
	mu       sync.Mutex
	count    int
	proc     *windows.LazyProc
}

// New — yangi Keeper yaratadi.
func New() *Keeper {
	kernel32 := windows.NewLazySystemDLL("kernel32.dll")
	return &Keeper{
		proc: kernel32.NewProc("SetThreadExecutionState"),
	}
}

// Hold — uxlashni bloklaydi.
func (k *Keeper) Hold() error {
	k.mu.Lock()
	defer k.mu.Unlock()
	k.count++
	if k.count == 1 {
		return k.setState(esContinuous | esSystemRequired | esAwayMode)
	}
	return nil
}

// Release — bitta hold'ni bekor qiladi.
func (k *Keeper) Release() error {
	k.mu.Lock()
	defer k.mu.Unlock()
	if k.count == 0 {
		return nil
	}
	k.count--
	if k.count == 0 {
		return k.setState(esContinuous) // faqat ES_CONTINUOUS — flag'larni tozalaydi
	}
	return nil
}

// IsHeld — hozirda uxlash bloklanganmi?
func (k *Keeper) IsHeld() bool {
	k.mu.Lock()
	defer k.mu.Unlock()
	return k.count > 0
}

// setState — SetThreadExecutionState API chaqirig'i.
func (k *Keeper) setState(flags uintptr) error {
	r, _, err := k.proc.Call(flags)
	if r == 0 {
		// SetThreadExecutionState 0 qaytarsa, xato yuz bergan
		return err
	}
	return nil
}
