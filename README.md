# 📹 Cam2You

> **Hikvision (va boshqa IP kamera) → YouTube Live va boshqa RTMP platformalarga jonli stream qiluvchi yengil Windows desktop dasturi.**
>
> A lightweight Windows desktop app for streaming Hikvision (and other IP cameras) to YouTube Live and other RTMP platforms.

[![Go](https://img.shields.io/badge/Go-1.23+-00ADD8?logo=go&logoColor=white)](https://go.dev)
[![Wails](https://img.shields.io/badge/Wails-v2.12-DF0000)](https://wails.io)
[![React](https://img.shields.io/badge/React-18-61DAFB?logo=react&logoColor=white)](https://react.dev)
[![License: MIT](https://img.shields.io/badge/License-MIT-yellow.svg)](LICENSE)

---

## ✨ Xususiyatlar / Features

- 🎥 **Hikvision, Dahua va istalgan RTSP kamerani** YouTube Live, Twitch, Facebook yoki custom RTMP'ga uzatish
- 🧩 **Multi-camera kompozitsiya**: 1×2 / 2×1 / 2×2 / 3×3 grid, Picture-in-Picture
- 🚀 **NVIDIA NVENC**, Intel QuickSync yoki CPU encoding (`-c:v copy` ham mumkin)
- 🔄 **Auto-restart** uzilgan strim'lar uchun (exponential backoff bilan)
- 👁 **Jonli ko'rish (HLS preview)** — kamerani dastur ichida ko'ring
- 📥 **FFmpeg avtomatik o'rnatish** (GitHub mirror + gyan.dev fallback)
- 💤 **Sleep prevention** — ekran o'chsa ham strim to'xtamaydi (`SetThreadExecutionState`)
- 🔔 **Windows Toast notifications**
- 🪟 **System tray** — X bossangiz tray'ga yashirinadi, strim ishlashda davom etadi
- 💾 **SQLite** — barcha kameralar va stream'lar lokal saqlanadi
- 📦 **Bitta `.exe`** — Python yoki Node runtime kerakmas (~15 MB)

---

## 🖼 Screenshots

> _(Skrinshot'lar qo'shilishi kerak — TODO)_

---

## 🚀 Tezkor boshlash / Quick start

### 1. Dasturni yuklab oling
[Releases](https://github.com/Yaxyobek0877/cam_to_you/releases) sahifasidan oxirgi `cam_to_you.exe` ni yuklang.

### 2. Ishga tushiring
Faylni ikki marta bosing — birinchi marta ishga tushganda:
- AppData papkasiga sozlamalar yoziladi (`%APPDATA%\Cam2You\`)
- Tray icon paydo bo'ladi

### 3. FFmpeg o'rnating
**Settings** sahifasi → **"Avtomatik yuklash va o'rnatish"** (~80-140 MB)
Yoki, agar internetingiz sekin bo'lsa, `gyan.dev` yoki `BtbN/FFmpeg-Builds` dan o'zingiz yuklab, **"Mavjud fayldan o'rnatish"** orqali ko'rsating.

### 4. Kamera qo'shing
**Cameras** → **"Yangi kamera"**
- Vendor: Hikvision / Dahua / Generic
- IP, port (554), login, parol
- **"Ulanishni tekshirish"** — codec va o'lcham ko'rinadi
- **"Jonli ko'rish"** — kamerani dastur ichida sinab ko'ring

### 5. Stream yarating va ishga tushiring
**Streams** → **"Yangi stream"**
- Layout tanlang (single, 2x2 grid, PiP, va h.k.)
- Kameralarni biriktiring
- Sifat (720p/1080p/1440p) va encoder
- YouTube Stream Key kiriting
- **"Ishga tushirish"** — YouTube Studio'da ko'rasiz

---

## 🏗 Arxitektura

```
┌────────────────────────────────────────────────┐
│         cam_to_you.exe (~15 MB)                │
│  ┌─────────────────────────────────────────┐   │
│  │  Native oyna (Wails + WebView2)         │   │
│  │  ┌───────────────────────────────────┐  │   │
│  │  │  React + TypeScript + Tailwind    │  │   │
│  │  │  TanStack Query + hls.js          │  │   │
│  │  └───────────────────────────────────┘  │   │
│  └─────────────────────────────────────────┘   │
│         ▲ Native IPC (HTTP server YO'Q)        │
│         ▼                                      │
│  ┌─────────────────────────────────────────┐   │
│  │  Go backend                             │   │
│  │  ├─ FFmpeg supervisor (goroutines)      │   │
│  │  ├─ Stream manager + auto-restart       │   │
│  │  ├─ Camera CRUD + RTSP probe            │   │
│  │  ├─ Systray (close-to-tray)             │   │
│  │  ├─ Sleep prevention (Win32 API)        │   │
│  │  ├─ Toast notifications                 │   │
│  │  ├─ HLS preview server                  │   │
│  │  └─ SQLite (pure Go, no CGo)            │   │
│  └─────────────────────────────────────────┘   │
└────────────────────────────────────────────────┘
        │                                  │
        │ RTSP (lokal)             RTMP (Internet)
        ▼                                  ▼
   📷 Kameralar              📡 YouTube / Twitch / FB
```

### Texnologiyalar / Tech stack

| Qatlam | Tanlov |
|--------|--------|
| Backend | Go 1.23+ |
| Desktop framework | [Wails v2](https://wails.io) (Go + WebView2, **server-less**) |
| Frontend | React 18 + TypeScript + Vite |
| UI | Tailwind CSS + lucide-react |
| State | [@tanstack/react-query](https://tanstack.com/query) |
| DB | [modernc.org/sqlite](https://modernc.org/sqlite) (pure Go, CGo kerakmas) |
| Streaming | FFmpeg (auto-install qilingan) |
| Systray | [energye/systray](https://github.com/energye/systray) |
| Notifications | [git.sr.ht/~jackmordaunt/go-toast/v2](https://git.sr.ht/~jackmordaunt/go-toast) |
| Video preview | HLS + [hls.js](https://github.com/video-dev/hls.js) |

---

## 🛠 Manbadan build qilish / Build from source

### Talablar
- Go 1.23+
- Node.js 18+
- [Wails CLI](https://wails.io/docs/gettingstarted/installation) — `go install github.com/wailsapp/wails/v2/cmd/wails@latest`
- Windows 10/11 + WebView2 (Win 11 da o'rnatilgan)

### Build
```powershell
git clone https://github.com/Yaxyobek0877/cam_to_you.git
cd cam_to_you
wails build
```

Natija: `build/bin/cam_to_you.exe`

### Development
```powershell
wails dev
```
Bu hot-reload bilan dev rejimda ishga tushiradi.

---

## 📁 Folder tuzilishi

```
cam_to_you/
├── main.go                          # Wails entry + close-to-tray
├── app.go                           # React API bindings
├── internal/
│   ├── config/                      # %APPDATA%\Cam2You yo'llari
│   ├── ffmpeg/
│   │   ├── installer.go             # Auto-install (multi-mirror)
│   │   ├── builder.go               # single/grid/PiP komandalarini quradi
│   │   └── runner.go                # Subprocess + graceful stop
│   ├── models/                      # Camera, Stream tiplari
│   ├── db/                          # SQLite + migrations
│   ├── camera/                      # CRUD + RTSP probe (ffprobe)
│   ├── stream/
│   │   ├── service.go               # CRUD
│   │   └── manager.go               # Supervisor + auto-restart
│   ├── preview/                     # RTSP → HLS bridge
│   ├── power/                       # SetThreadExecutionState
│   ├── notify/                      # Windows Toast
│   └── tray/                        # System tray
└── frontend/src/
    ├── App.tsx                      # Router + Query Client
    ├── components/                  # Layout, PreviewModal
    ├── lib/                         # types, api client, utils
    └── pages/
        ├── Dashboard.tsx
        ├── Cameras.tsx
        ├── Streams.tsx
        └── Settings.tsx
```

---

## 🎯 Yo'l xaritasi / Roadmap

- [x] Bitta kamera → YouTube
- [x] Bir nechta kamera (alohida streamlar)
- [x] Multi-camera grid kompozitsiya (1×2, 2×1, 2×2, 3×3, PiP)
- [x] Auto-restart
- [x] FFmpeg auto-installer + qo'lda variant
- [x] HLS jonli preview
- [x] System tray + close-to-tray
- [x] Sleep prevention
- [x] Windows toast notifications
- [ ] Lokal yozish (stream + MP4 fayl bir vaqtda)
- [ ] Watermark / logo overlay
- [ ] ONVIF avtomatik kamera topish
- [ ] Stream jadval bo'yicha (cron)
- [ ] Telegram/Email orqali xato bildirishnomalari
- [ ] PTZ boshqaruv
- [ ] Multi-platform simultaneously (YouTube + Twitch)
- [ ] Foydalanuvchi rollari (admin/operator)
- [ ] macOS / Linux qo'llab-quvvatlash

---

## 🤝 Hissa qo'shish / Contributing

Pull request'lar xush kelibsiz! Katta o'zgarishlar uchun avval issue oching, muhokama qilamiz.

```bash
git checkout -b feature/yangi-narsa
# o'zgarishlar...
git commit -m "feat: yangi narsa qo'shildi"
git push origin feature/yangi-narsa
```

---

## 📄 Litsenziya / License

MIT — qarang [LICENSE](LICENSE).

---

## 🙏 Minnatdorchilik / Acknowledgements

- [FFmpeg](https://ffmpeg.org) — streaming yadrosi
- [Wails](https://wails.io) — Go + Web bilan desktop dastur yaratish
- [Gyan.dev FFmpeg builds](https://www.gyan.dev/ffmpeg/builds/) — Windows binary'lar
- [BtbN/FFmpeg-Builds](https://github.com/BtbN/FFmpeg-Builds) — GitHub mirror

---

**Muammo yoki taklif bormi?** [Issue oching](https://github.com/Yaxyobek0877/cam_to_you/issues).
