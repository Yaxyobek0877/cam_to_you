/**
 * Global event store — barcha Go'dan kelgan hodisalarni vaqtli tartibda saqlaydi.
 * Logs sahifasi shu store'dan o'qiydi.
 *
 * Foydalanish:
 *   1. App boot'ida initEventStore() chaqiriladi
 *   2. Hodisalar avtomatik ringbuffer'ga yoziladi (oxirgi 1000)
 *   3. UI komponentlari useEventLog() bilan reaktiv tinglashlari mumkin
 */

import { onStreamEvent, onPreviewEvent, onFFmpegProgress } from "./api";

export type LogSource = "stream" | "preview" | "ffmpeg" | "system";
export type LogLevel = "info" | "warning" | "error";

export interface LogEntry {
  id: number;
  time: Date;
  source: LogSource;
  sourceId?: number;     // streamId yoki cameraId
  level: LogLevel;
  type: string;          // event turi: state_change, log, error, ready, ...
  message: string;
  raw?: any;             // asl payload (debug uchun)
}

const MAX_ENTRIES = 1000;
let entries: LogEntry[] = [];
let nextId = 1;
let listeners: Array<(entries: LogEntry[]) => void> = [];

function add(entry: Omit<LogEntry, "id">) {
  const full: LogEntry = { ...entry, id: nextId++ };
  entries = [...entries, full];
  if (entries.length > MAX_ENTRIES) {
    entries = entries.slice(entries.length - MAX_ENTRIES);
  }
  listeners.forEach((l) => l(entries));
}

export function getEntries(): LogEntry[] {
  return entries;
}

export function clearEntries() {
  entries = [];
  listeners.forEach((l) => l(entries));
}

export function subscribe(listener: (entries: LogEntry[]) => void): () => void {
  listeners.push(listener);
  return () => {
    listeners = listeners.filter((l) => l !== listener);
  };
}

let initialized = false;
export function initEventStore() {
  if (initialized) return;
  initialized = true;

  // Boshlanish hodisasi
  add({
    time: new Date(),
    source: "system",
    level: "info",
    type: "boot",
    message: "Cam2You ishga tushdi",
  });

  // Stream hodisalari
  onStreamEvent((data: any) => {
    const ev = data as { type: string; streamId: number; payload?: any };
    let level: LogLevel = "info";
    let message = "";

    switch (ev.type) {
      case "state_change":
        message = `Holat: ${ev.payload}`;
        if (ev.payload === "error") level = "error";
        if (ev.payload === "running") level = "info";
        break;
      case "log":
        level = (ev.payload?.level as LogLevel) || "info";
        message = ev.payload?.message || "(bo'sh log)";
        break;
      case "error":
        level = "error";
        message = typeof ev.payload === "string" ? ev.payload : JSON.stringify(ev.payload);
        break;
      case "restart":
        level = "warning";
        message = `Qayta urinish #${ev.payload}`;
        break;
      case "live": {
        // Birinchi frame YouTube'ga uzatildi — foydalanuvchini darrov xabardor qilamiz.
        // Bu MUHIM: aks holda foydalanuvchi 2 ta warning ko'rib, "ulanmadi" deb o'ylaydi.
        const p = ev.payload || {};
        add({
          time: new Date(),
          source: "stream",
          sourceId: ev.streamId,
          level: "info",
          type: "live",
          message: p.message || "✅ Stream YouTube'ga uzatilmoqda",
          raw: ev,
        });
        return;
      }
      case "progress": {
        // FFmpeg statistikasi (har 5 sek'da): foydalanuvchi haqiqatdan ishlayotganini ko'radi.
        const p = ev.payload || {};
        const fps = parseFloat(p.fps || "0");
        const bitrate = p.bitrate || "?";
        const timeCode = p.time || "?";
        add({
          time: new Date(),
          source: "stream",
          sourceId: ev.streamId,
          level: "info",
          type: "progress",
          message: `📡 ${fps.toFixed(0)} fps · ${bitrate} · ${timeCode} efirda`,
          raw: ev,
        });
        return;
      }
      case "exit_reason": {
        // FFmpeg chiqqanida oxirgi 8 qator — clean exit'da ham sababini ko'rsatadi
        const p = ev.payload || {};
        const exitCode = p.exitCode ?? "?";
        const hint: string = p.hint || "";
        const lines: Array<{ message: string; level?: string }> = p.lines || [];
        level = exitCode === 0 ? "warning" : "error";
        // Birinchi qator — qisqa summary
        const summary = `🔍 FFmpeg chiqdi (exit=${exitCode}, state=${p.state || "?"}). Oxirgi qatorlar:`;
        add({
          time: new Date(),
          source: "stream",
          sourceId: ev.streamId,
          level,
          type: "exit_reason",
          message: summary,
          raw: ev,
        });
        // Agar aniq tavsiya topilgan bo'lsa, uni alohida kuchli (warning) qatorda ko'rsatamiz
        if (hint) {
          add({
            time: new Date(),
            source: "stream",
            sourceId: ev.streamId,
            level: "warning",
            type: "exit_hint",
            message: hint,
            raw: { hint },
          });
        }
        // Har bir log qatorini alohida entry qilib qo'shamiz — UI'da yaxshi ko'rinadi
        for (const ln of lines) {
          const lnLevel: LogLevel =
            ln.level === "error" ? "error" : ln.level === "warning" ? "warning" : "info";
          add({
            time: new Date(),
            source: "ffmpeg",
            sourceId: ev.streamId,
            level: lnLevel,
            type: "exit_log",
            message: `  └ ${ln.message}`,
            raw: ln,
          });
        }
        return; // exit_reason'ni alohida tarzda qayta ishladik
      }
      default:
        message = ev.type;
    }

    add({
      time: new Date(),
      source: "stream",
      sourceId: ev.streamId,
      level,
      type: ev.type,
      message,
      raw: ev,
    });
  });

  // Preview hodisalari
  onPreviewEvent((data: any) => {
    const ev = data as { type: string; cameraId: number; payload?: any };
    let level: LogLevel = "info";
    let message = "";

    switch (ev.type) {
      case "starting":
        message = `Preview boshlandi (encoder: ${ev.payload?.encoder || "?"})`;
        break;
      case "ready":
        message = "Video oqimi tayyor";
        break;
      case "log":
        level = (ev.payload?.level as LogLevel) || "info";
        message = ev.payload?.message || "(bo'sh log)";
        break;
      case "error":
        level = "error";
        message = typeof ev.payload === "string" ? ev.payload : JSON.stringify(ev.payload);
        break;
      case "stopped":
        message = "Preview to'xtatildi";
        break;
      default:
        message = ev.type;
    }

    add({
      time: new Date(),
      source: "preview",
      sourceId: ev.cameraId,
      level,
      type: ev.type,
      message,
      raw: ev,
    });
  });

  // FFmpeg progress (auto-installer)
  onFFmpegProgress((data: any) => {
    const ev = data as { stage: string; percent: number; speedMBps: number; message: string };
    if (ev.stage === "done") {
      add({
        time: new Date(),
        source: "ffmpeg",
        level: "info",
        type: "install_done",
        message: ev.message,
        raw: ev,
      });
    }
  });
}
