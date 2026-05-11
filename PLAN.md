# 📹 Cam2You — Hikvision IP Camera → YouTube Live Streaming Platform

> **Maqsad:** Lokal tarmoqdagi Hikvision IP kameralardan video oqimini olib, YouTube Live'ga (va boshqa RTMP platformalarga) translyatsiya qiluvchi lokal dastur yaratish. Bir nechta kamerani bitta stream'ga birlashtirish yoki har birini alohida streamga yuborish imkoniyati bilan.

---

## 1. Loyiha Asosiy Maqsadi (Project Scope)

### MVP (Minimum Viable Product) — 1-versiya
- ✅ Bitta Hikvision kamerani YouTube'ga stream qilish
- ✅ Bir nechta kamerani **alohida** YouTube streamlarga yuborish (har biriga alohida stream key)
- ✅ Bir nechta kamerani **bitta** stream ichida grid/mozaika ko'rinishida birlashtirish (2x1, 2x2, 3x3 va h.k.)
- ✅ Web UI (brauzerdan boshqarish)
- ✅ Kameralar va streamlarni saqlash (config persistence)
- ✅ Stream holatini real-time kuzatish (running / stopped / error)

### V2 — Kengaytirilgan funksiyalar
- 🎯 Avtomatik qayta ulanish (kamera yoki YouTube uzilsa)
- 🎯 Lokal yozish (stream paytida MP4 ga ham saqlash)
- 🎯 Logo/watermark qo'yish
- 🎯 Stream preview (jonli oldindan ko'rish, brauzerdan)
- 🎯 ONVIF orqali kameralarni avtomatik topish
- 🎯 Bitrate/resolution boshqaruvi
- 🎯 Audio on/off / mikser

### V3 — Premium
- 🚀 Bir vaqtning o'zida YouTube + Twitch + Facebook (multi-platform)
- 🚀 Jadval bo'yicha stream (planlashtirish)
- 🚀 Telegram/Email orqali xato bildirishnomalari
- 🚀 PTZ kameralarni boshqarish
- 🚀 Foydalanuvchi rollari (admin/operator)

---

## 2. Texnik Arxitektura (High-Level)

```
┌────────────────────────────────────────────────────────────┐
│                    WEB UI (Brauzer)                        │
│  - Kamera ro'yxati  - Stream paneli  - Live preview        │
└──────────────────────┬─────────────────────────────────────┘
                       │  REST API + WebSocket (real-time)
                       │
┌──────────────────────▼─────────────────────────────────────┐
│              BACKEND ORCHESTRATOR (FastAPI)                │
│  ┌──────────────┐ ┌─────────────┐ ┌──────────────────┐    │
│  │ Camera Mgr   │ │ Stream Mgr  │ │  Health Monitor  │    │
│  └──────────────┘ └─────────────┘ └──────────────────┘    │
│  ┌──────────────┐ ┌─────────────┐ ┌──────────────────┐    │
│  │ FFmpeg Pool  │ │ Config (DB) │ │  Event Bus       │    │
│  └──────┬───────┘ └─────────────┘ └──────────────────┘    │
└─────────┼──────────────────────────────────────────────────┘
          │ spawn/kill/monitor
          │
┌─────────▼──────────────────────────────────────────────────┐
│              FFMPEG WORKER PROCESSES                       │
│  Worker 1: cam1 → YT stream A                              │
│  Worker 2: cam2 → YT stream B                              │
│  Worker 3: (cam1 + cam2 + cam3 + cam4 grid) → YT stream C  │
└─────────┬────────────────────────────────┬─────────────────┘
          │ RTSP IN                        │ RTMP OUT
          ▼                                ▼
   ┌─────────────┐                ┌──────────────────┐
   │  Hikvision  │                │   YouTube Live   │
   │  Cameras    │                │   (or Twitch...) │
   └─────────────┘                └──────────────────┘
```

### Asosiy oqim (data flow)
1. **Kamera input:** `rtsp://user:pass@192.168.1.X:554/Streaming/Channels/101`
2. **FFmpeg worker** RTSP oqimini oladi → kerakli format/codec'ga o'tkazadi → RTMP push qiladi
3. **YouTube output:** `rtmp://a.rtmp.youtube.com/live2/{STREAM_KEY}`

---

## 3. Texnologiyalar (Tech Stack)

| Qatlam | Tanlov | Sabab |
|--------|--------|-------|
| **Streaming engine** | **FFmpeg** | De-facto standart, RTSP+RTMP+filter_complex (grid uchun) qo'llab-quvvatlaydi |
| **Backend** | **Python 3.11 + FastAPI** | Async, oson FFmpeg subprocess boshqaruvi, ONVIF kutubxonalari (`onvif-zeep`) mavjud |
| **Process manager** | `asyncio.subprocess` + custom supervisor | Har bir stream uchun alohida FFmpeg process |
| **DB** | **SQLite** + SQLAlchemy | Lokal app uchun yetarli, qo'shimcha server kerakmas |
| **Frontend** | **React + Vite + TailwindCSS** | Tez UI, Vite quick dev |
| **Real-time UI** | **WebSocket** (FastAPI native) | Stream status, log streaming |
| **Live preview (V2)** | HLS yoki WebRTC (MediaMTX yordamida) | Brauzerda RTSP'ni to'g'ridan to'g'ri ko'rib bo'lmaydi |
| **Paketlash** | **PyInstaller** yoki **Docker** | Lokal o'rnatish uchun yagona `.exe` yoki konteyner |
| **Auth** | JWT (lokal foydalanuvchilar) | V2 uchun |

### Alternativalar (nima uchun **emas**)
- **Node.js**: ishlaydi, lekin Pythonda ONVIF/RTSP utilitalari ko'proq.
- **GStreamer**: kuchliroq, lekin FFmpeg'dan murakkabroq — overkill MVP uchun.
- **Nginx-RTMP module**: relay uchun yaxshi, lekin grid (compositing) qila olmaydi → FFmpeg baribir kerak.

---

## 4. Funksional Talablar (Features Breakdown)

### 4.1 Kamera Boshqaruvi
- [ ] Qo'lda kamera qo'shish (nom, IP, port, username, password, channel)
- [ ] RTSP URL'ni avtomatik yasash (Hikvision shabloni: `/Streaming/Channels/{ch}01`)
- [ ] "Test connection" tugmasi → 5 soniyalik snapshot oling
- [ ] Kameralar ro'yxati (online/offline indikator)
- [ ] Tahrirlash / O'chirish
- [ ] **(V2)** ONVIF orqali tarmoqdan avtomatik scan

### 4.2 Stream Boshqaruvi
- [ ] Yangi stream yaratish:
  - Nomi
  - Tip: `single` (1 kamera) yoki `composite` (bir nechta kamera grid'da)
  - Manba kameralar (1 yoki ko'p)
  - Layout (faqat composite uchun): `1x1`, `1x2`, `2x2`, `3x3`, `picture-in-picture`
  - Chiqish: YouTube stream key (yoki custom RTMP URL)
  - Sifat: resolution (720p/1080p), bitrate (2500/4500/6000 kbps), FPS (25/30)
  - Audio: kameradan / o'chirilgan / custom audio fayldan
- [ ] Start / Stop / Restart tugmalari
- [ ] Bir vaqtning o'zida bir nechta stream ishlashi (CPU/GPU imkon qadar)

### 4.3 Multi-Kamera Stsenariylari
| Stsenariy | Misol | FFmpeg yondashuvi |
|-----------|-------|------------------|
| **1 → 1** | cam1 → YT-A | Oddiy `-i rtsp -c copy -f flv rtmp://...` |
| **N → N (alohida)** | cam1→YT-A, cam2→YT-B | N ta alohida FFmpeg process |
| **N → 1 (grid)** | cam1+cam2+cam3+cam4 → YT-C (2x2) | Bitta FFmpeg, `-filter_complex xstack` |
| **N → 1 (PiP)** | cam1 (asosiy) + cam2 (burchakda) → YT-C | `overlay` filter |
| **1 → N** | cam1 → YT-A & YT-B (relay) | `tee` muxer yoki nginx-rtmp relay |

### 4.4 Monitoring & Health
- [ ] Real-time stream holati (running/error/reconnecting)
- [ ] FFmpeg log oqimi UI'da ko'rinadi
- [ ] CPU/RAM ishlatish ko'rsatkichi
- [ ] Stream uptime
- [ ] Avtomatik qayta ulanish (kamera uzilganda, exponential backoff)
- [ ] **(V2)** Dropped frames, bitrate haqiqiy ko'rsatkich

### 4.5 Qo'shimchalar
- [ ] **Lokal yozish**: stream paytida MP4 ga ham yozish (`tee` filter)
- [ ] **Watermark/Logo**: PNG'ni burchakka qo'yish
- [ ] **Vaqt/sana overlay**
- [ ] **Stream jadvali**: "Har kuni 09:00 - 18:00 ishlasin"
- [ ] **YouTube API integratsiyasi (V3)**: stream key'ni avtomatik olish, broadcast yaratish

---

## 5. Loyiha Tuzilishi (Folder Structure)

```
cam_to_you/
├── PLAN.md                    ← shu fayl
├── README.md
├── docker-compose.yml         ← (ixtiyoriy) FFmpeg + app birga
├── pyproject.toml             ← Python deps
│
├── backend/
│   ├── app/
│   │   ├── main.py            ← FastAPI entry
│   │   ├── api/
│   │   │   ├── cameras.py     ← /api/cameras CRUD
│   │   │   ├── streams.py     ← /api/streams CRUD + start/stop
│   │   │   └── ws.py          ← WebSocket: stream events
│   │   ├── core/
│   │   │   ├── config.py
│   │   │   ├── db.py          ← SQLite session
│   │   │   └── models.py      ← Camera, Stream SQLAlchemy
│   │   ├── services/
│   │   │   ├── ffmpeg_runner.py   ← FFmpeg subprocess wrapper
│   │   │   ├── stream_manager.py  ← supervisor: spawn/kill/monitor
│   │   │   ├── ffmpeg_builder.py  ← command builder (single/grid/PiP)
│   │   │   ├── camera_probe.py    ← RTSP ulanishini test qilish
│   │   │   └── onvif_scanner.py   ← (V2) tarmoq scan
│   │   └── schemas/            ← Pydantic
│   └── tests/
│
├── frontend/
│   ├── index.html
│   ├── vite.config.ts
│   ├── package.json
│   └── src/
│       ├── App.tsx
│       ├── pages/
│       │   ├── Cameras.tsx
│       │   ├── Streams.tsx
│       │   └── Dashboard.tsx
│       ├── components/
│       │   ├── CameraCard.tsx
│       │   ├── StreamCard.tsx
│       │   ├── LayoutPicker.tsx   ← grid layout tanlash
│       │   └── LiveLogs.tsx
│       └── api/client.ts
│
├── data/                      ← .gitignore'da
│   ├── app.db                 ← SQLite
│   └── recordings/            ← lokal yozuvlar
│
└── scripts/
    ├── install.ps1
    └── ffmpeg_setup.md        ← FFmpeg o'rnatish bo'yicha qo'llanma
```

---

## 6. Asosiy FFmpeg Buyruqlari (Texnik Yadro)

### Oddiy 1→1 (eng asosiy)
```bash
ffmpeg -rtsp_transport tcp -i rtsp://admin:pass@192.168.1.64:554/Streaming/Channels/101 \
       -c:v copy -c:a aac -b:a 128k -f flv \
       rtmp://a.rtmp.youtube.com/live2/STREAM_KEY
```
> `-c:v copy` — qayta kodlamaydi (CPU saqlaydi). Agar resolution o'zgartirilsa, `libx264` ishlatamiz.

### 2x2 grid (4 kamera → 1 stream)
```bash
ffmpeg \
  -rtsp_transport tcp -i rtsp://...cam1 \
  -rtsp_transport tcp -i rtsp://...cam2 \
  -rtsp_transport tcp -i rtsp://...cam3 \
  -rtsp_transport tcp -i rtsp://...cam4 \
  -filter_complex "[0:v][1:v][2:v][3:v]xstack=inputs=4:layout=0_0|w0_0|0_h0|w0_h0[v]" \
  -map "[v]" -c:v libx264 -preset veryfast -b:v 4500k \
  -f flv rtmp://a.rtmp.youtube.com/live2/STREAM_KEY
```

### PiP (Picture-in-Picture)
```bash
-filter_complex "[1:v]scale=320:180[pip];[0:v][pip]overlay=W-w-10:H-h-10[v]"
```

### Stream + lokal yozish bir vaqtda
```bash
-f tee "[f=flv]rtmp://...|[f=mp4]/data/recordings/stream_$(date).mp4"
```

---

## 7. Rivojlanish Bosqichlari (Roadmap)

### 🏁 Bosqich 1: Skelet (1-hafta)
- [ ] Repo init, folder structure
- [ ] FastAPI bootstrap, SQLite + modellar
- [ ] FFmpeg subprocess wrapper (start/stop/log capture)
- [ ] CLI orqali 1 ta kamerani YouTube'ga stream qilish (UI'siz)

### 🎨 Bosqich 2: Web UI MVP (1-hafta)
- [ ] React + Vite setup
- [ ] Kamera CRUD UI
- [ ] Stream CRUD UI (faqat `single` tip)
- [ ] Start/Stop tugmalari ishlaydi
- [ ] WebSocket orqali status real-time

### 🧩 Bosqich 3: Multi-cam (1-hafta)
- [ ] `ffmpeg_builder.py` da `composite` (xstack/PiP) qo'llab-quvvatlash
- [ ] UI'da layout picker komponenti (2x2, 1x2, PiP)
- [ ] Bir nechta alohida streamni parallel ishga tushirish stress-test

### 🛡 Bosqich 4: Stability (1-hafta)
- [ ] Auto-reconnect (kamera uzilsa exponential backoff)
- [ ] Health monitor (CPU, RAM, uptime)
- [ ] Log viewer UI
- [ ] Error handling + xato bildirishnomalari

### ✨ Bosqich 5: Qo'shimchalar (rolling)
- [ ] Live preview (HLS via MediaMTX)
- [ ] Lokal yozish
- [ ] Logo/watermark
- [ ] ONVIF auto-discovery
- [ ] Multi-platform (Twitch, FB)

### 📦 Bosqich 6: Paketlash
- [ ] PyInstaller `.exe` (Windows)
- [ ] Yoki Docker image
- [ ] O'rnatish qo'llanmasi

---

## 8. Asosiy Risklar va Yechimlar

| Risk | Ehtimol | Yechim |
|------|---------|--------|
| **CPU yetishmaslik** (ko'p stream + kodlash) | Yuqori | `-c:v copy` ishlatish, GPU encoding (NVENC), resolution kamaytirish |
| **Tarmoq band kengligi** (upload) | Yuqori | Bitrate sozlash, sub-stream ishlatish (`/102`) |
| **YouTube uzilishi** | O'rta | Auto-restart, exponential backoff |
| **Kamera login parol noto'g'ri** | Yuqori | "Test connection" tugmasi, aniq xato xabari |
| **FFmpeg yo'q yoki noto'g'ri versiya** | O'rta | App startda tekshirish, `scripts/ffmpeg_setup.md` |
| **Audio sinxron emas** | O'rta | `-async 1` yoki `-vsync cfr` flaglari |
| **YouTube Stream Key sizib chiqishi** | O'rta | DB'da shifrlangan saqlash (Fernet), UI'da yashirish |
| **Multi-stream RAM leak** | Past | Har bir worker uchun cgroup/memory limit (V2) |

---

## 9. Aniqlashtirish Kerak Bo'lgan Savollar

Loyihani boshlashdan oldin, iltimos quyidagilarni tasdiqlang:

1. **Platform**: Faqat Windows'da ishlashi kerakmi, yoki Linux/Mac ham?
2. **Foydalanuvchilar**: Yagona foydalanuvchimi yoki ko'p foydalanuvchili (login bilan)?
3. **GPU**: Server/PC'da NVIDIA GPU bormi? (NVENC kodlash uchun 5-10x tezroq)
4. **Maksimal kameralar soni**: Taxminan nechta kamera bir vaqtda ishlaydi? (4? 8? 16?)
5. **Internet upload tezligi**: Necha Mbps? (1080p@30fps ≈ 5 Mbps upload)
6. **YouTube hisob**: Ko'p kanalga streamingmi yoki bitta kanalga?
7. **UI til**: O'zbek tili (lotin/kirill) kerakmi yoki ingliz tilida boshlaymizmi?
8. **Yozib olish**: Lokalga saqlash kerakmi yoki faqat translyatsiya?

---

## 10. Keyingi Qadam

Tasdiqlasangiz, men:
1. **Bosqich 1**'dan boshlab kod yozaman (FastAPI skeleti + bitta kameradan YouTube'ga stream qiluvchi CLI demo)
2. Yoki, agar xohlasangiz, avval **prototip**: 50 qatorlik Python skript bilan 1 ta kamerani YouTube'ga uzatib ko'ramiz — ishlasa, keyin to'liq arxitekturani quramiz

Qaysi yo'lni xohlaysiz?

---

*Bu reja boshlang'ich versiya — har qanday qismini o'zgartirishimiz yoki kengaytirishimiz mumkin.*
