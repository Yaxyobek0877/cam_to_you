// Bu tool: SVG faylidan 2 ta narsa ishlab chiqaradi:
//
//  1. build/appicon.png       — 1024×1024 PNG (Wails uchun)
//  2. build/windows/icon.ico  — Windows ICO (16, 32, 48, 64, 128, 256 o'lchamli)
//
// Foydalanish:
//
//	cd tools/icongen
//	go run .
package main

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"image"
	"image/png"
	"os"
	"path/filepath"

	"github.com/srwiley/oksvg"
	"github.com/srwiley/rasterx"
)

// Windows .ico format uchun ko'rsatilgan o'lchamlar.
// Windows Explorer bularning eng mosini tanlaydi.
var icoSizes = []int{16, 24, 32, 48, 64, 128, 256}

// PNG ekspozitsiya o'lchami — Wails appicon.png uchun.
const appIconSize = 1024

func main() {
	root, _ := filepath.Abs("../..")
	svgPath := filepath.Join(root, "build", "icon.svg")
	pngPath := filepath.Join(root, "build", "appicon.png")
	icoPath := filepath.Join(root, "build", "windows", "icon.ico")

	fmt.Printf("→ SVG manba: %s\n", svgPath)

	// 1) 1024×1024 PNG (Wails)
	bigPNG, err := renderSVG(svgPath, appIconSize)
	if err != nil {
		fail("PNG render: %v", err)
	}
	if err := os.WriteFile(pngPath, bigPNG, 0o644); err != nil {
		fail("appicon.png yozish: %v", err)
	}
	fmt.Printf("✓ Yozildi: %s (%d×%d, %d bayt)\n", pngPath, appIconSize, appIconSize, len(bigPNG))

	// 2) Multi-size ICO (Windows)
	images := make([][]byte, len(icoSizes))
	for i, sz := range icoSizes {
		img, err := renderSVG(svgPath, sz)
		if err != nil {
			fail("render %dpx: %v", sz, err)
		}
		images[i] = img
		fmt.Printf("  + %d×%d PNG (%d bayt)\n", sz, sz, len(img))
	}

	icoBytes := buildICO(icoSizes, images)
	if err := os.MkdirAll(filepath.Dir(icoPath), 0o755); err != nil {
		fail("windows/ papkasi: %v", err)
	}
	if err := os.WriteFile(icoPath, icoBytes, 0o644); err != nil {
		fail("icon.ico yozish: %v", err)
	}
	fmt.Printf("✓ Yozildi: %s (%d bayt, %d o'lcham)\n", icoPath, len(icoBytes), len(icoSizes))
}

// renderSVG — SVG'ni berilgan o'lchamdagi PNG bayt massivga rasterizatsiya qiladi.
func renderSVG(svgPath string, size int) ([]byte, error) {
	icon, err := oksvg.ReadIcon(svgPath, oksvg.StrictErrorMode)
	if err != nil {
		return nil, err
	}
	img := image.NewRGBA(image.Rect(0, 0, size, size))
	icon.SetTarget(0, 0, float64(size), float64(size))
	scanner := rasterx.NewScannerGV(size, size, img, img.Bounds())
	raster := rasterx.NewDasher(size, size, scanner)
	icon.Draw(raster, 1.0)

	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// buildICO — Windows ICO formatida bir nechta PNG'ni bitta faylga to'playdi.
//
// ICO format tuzilishi:
//
//	ICONDIR (6 bayt)              — header
//	ICONDIRENTRY × N (16 bayt)    — har bir image uchun
//	Image data...                 — PNG yoki BMP baytlari ketma-ket
//
// Zamonaviy Windows (Vista+) ICO ichidagi PNG'ni qabul qiladi — biz shundan
// foydalanamiz (BMP'dan kichikroq).
func buildICO(sizes []int, pngs [][]byte) []byte {
	const headerSize = 6
	const entrySize = 16

	count := len(sizes)
	dataOffset := headerSize + entrySize*count

	var buf bytes.Buffer

	// ICONDIR
	binary.Write(&buf, binary.LittleEndian, uint16(0)) // Reserved
	binary.Write(&buf, binary.LittleEndian, uint16(1)) // Type = 1 (ICO)
	binary.Write(&buf, binary.LittleEndian, uint16(count))

	// ICONDIRENTRY × N
	currentOffset := uint32(dataOffset)
	for i, sz := range sizes {
		w := byte(sz)
		h := byte(sz)
		if sz >= 256 {
			w = 0 // 256 deb 0 yoziladi
			h = 0
		}
		buf.WriteByte(w)
		buf.WriteByte(h)
		buf.WriteByte(0) // ColorCount (256+ ranglar uchun 0)
		buf.WriteByte(0) // Reserved
		binary.Write(&buf, binary.LittleEndian, uint16(1))  // Planes
		binary.Write(&buf, binary.LittleEndian, uint16(32)) // BitCount
		binary.Write(&buf, binary.LittleEndian, uint32(len(pngs[i])))
		binary.Write(&buf, binary.LittleEndian, currentOffset)
		currentOffset += uint32(len(pngs[i]))
	}

	// Image data
	for _, p := range pngs {
		buf.Write(p)
	}

	return buf.Bytes()
}

func fail(format string, args ...interface{}) {
	fmt.Fprintf(os.Stderr, "✗ "+format+"\n", args...)
	os.Exit(1)
}
